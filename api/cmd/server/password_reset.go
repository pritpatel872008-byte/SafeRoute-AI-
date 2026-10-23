package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type resetRequest struct { Email string `json:"email"` }
type resetConfirm struct { Email string `json:"email"`; Token string `json:"token"`; Password string `json:"new_password"` }

func (s *server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) { return }
	apiKey:=strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	from:=strings.TrimSpace(os.Getenv("MAIL_FROM"))
	baseURL:=strings.TrimSpace(os.Getenv("PASSWORD_RESET_URL"))
	if apiKey=="" || from=="" || baseURL=="" {
		writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"password_reset_email_not_configured","detail":"Configure RESEND_API_KEY, MAIL_FROM (verified sender), and PASSWORD_RESET_URL on the API before requesting password reset emails."})
		return
	}
	var input resetRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,8<<10)).Decode(&input);err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"});return }
	input.Email=strings.ToLower(strings.TrimSpace(input.Email))
	if len(input.Email)<5 || len(input.Email)>254 || !strings.Contains(input.Email,"@") { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"valid_email_required"});return }
	var userID string
	err:=s.db.QueryRow(r.Context(),`SELECT id FROM app_users WHERE email=$1`,input.Email).Scan(&userID)
	if err!=nil {
		// Do not reveal whether an account exists.
		writeJSON(w,http.StatusAccepted,map[string]string{"message":"If an account exists for that address, a reset email will be sent."})
		return
	}
	raw:=make([]byte,32)
	if _,err=rand.Read(raw);err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_token_generation_failed"});return }
	token:=hex.EncodeToString(raw);sum:=sha256.Sum256([]byte(token))
	_,err=s.db.Exec(r.Context(),`INSERT INTO password_reset_tokens(user_id,token_hash,expires_at) VALUES($1,$2,$3)`,userID,hex.EncodeToString(sum[:]),time.Now().Add(30*time.Minute))
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_token_store_failed"});return }
	resetURL:=baseURL
	separator:="?";if strings.Contains(resetURL,"?"){separator="&"}
	resetURL+=separator+"email="+url.QueryEscape(input.Email)+"&token="+url.QueryEscape(token)
	body:=map[string]any{"from":from,"to":[]string{input.Email},"subject":"Reset your SafeRoute AI password","text":"A password reset was requested for your SafeRoute AI account. This link expires in 30 minutes. If you did not request it, ignore this email.\n\n"+resetURL,"html":"<p>A password reset was requested for your SafeRoute AI account.</p><p>This link expires in 30 minutes. If you did not request it, ignore this email.</p><p><a href=\""+resetURL+"\">Reset password</a></p>"}
	payload,err:=json.Marshal(body)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_email_encode_failed"});return }
	ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.resend.com/emails",bytes.NewReader(payload))
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_email_request_failed"});return }
	req.Header.Set("Authorization","Bearer "+apiKey);req.Header.Set("Content-Type","application/json")
	client:=&http.Client{Timeout:10*time.Second}
	resp,err:=client.Do(req)
	if err!=nil { _,_=s.db.Exec(r.Context(),`DELETE FROM password_reset_tokens WHERE token_hash=$1`,hex.EncodeToString(sum[:]));writeJSON(w,http.StatusBadGateway,map[string]string{"error":"reset_email_provider_unreachable"});return }
	defer resp.Body.Close()
	if resp.StatusCode<200 || resp.StatusCode>=300 { _,_=s.db.Exec(r.Context(),`DELETE FROM password_reset_tokens WHERE token_hash=$1`,hex.EncodeToString(sum[:]));writeJSON(w,http.StatusBadGateway,map[string]string{"error":"reset_email_provider_rejected_request","provider_status":fmt.Sprint(resp.StatusCode)});return }
	writeJSON(w,http.StatusAccepted,map[string]string{"message":"If an account exists for that address, a reset email will be sent."})
}

func (s *server) confirmPasswordReset(w http.ResponseWriter,r *http.Request) {
	if !s.requireDB(w) { return }
	var input resetConfirm
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,8<<10)).Decode(&input);err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"});return }
	input.Email=strings.ToLower(strings.TrimSpace(input.Email));input.Token=strings.TrimSpace(input.Token)
	if len(input.Email)<5 || len(input.Email)>254 || !strings.Contains(input.Email,"@") || len(input.Password)<12 || len(input.Password)>72 || len(input.Token)!=64 {
		writeJSON(w,http.StatusBadRequest,map[string]string{"error":"valid_email_token_and_12_to_72_byte_password_required"});return
	}
	sum:=sha256.Sum256([]byte(input.Token));hash:=hex.EncodeToString(sum[:])
	tx,err:=s.db.Begin(r.Context());if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_transaction_start_failed"});return };defer tx.Rollback(r.Context())
	var userID string
	err=tx.QueryRow(r.Context(),`SELECT pr.user_id FROM password_reset_tokens pr JOIN app_users u ON u.id=pr.user_id WHERE u.email=$1 AND pr.token_hash=$2 AND pr.expires_at>now() AND pr.used_at IS NULL FOR UPDATE OF pr`,input.Email,hash).Scan(&userID)
	if err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"reset_token_invalid_or_expired"});return }
	passwordHash,err:=bcrypt.GenerateFromPassword([]byte(input.Password),bcrypt.DefaultCost)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"password_hash_failed"});return }
	if _,err=tx.Exec(r.Context(),`UPDATE app_users SET password_hash=$1,updated_at=now() WHERE id=$2`,string(passwordHash),userID);err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"password_update_failed"});return }
	if _,err=tx.Exec(r.Context(),`UPDATE password_reset_tokens SET used_at=now() WHERE id IN (SELECT id FROM password_reset_tokens WHERE user_id=$1)`,userID);err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"reset_token_consume_failed"});return }
	if _,err=tx.Exec(r.Context(),`DELETE FROM sessions WHERE user_id=$1`,userID);err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"session_revoke_failed"});return }
	if err=tx.Commit(r.Context());err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"password_reset_commit_failed"});return }
	writeJSON(w,http.StatusOK,map[string]string{"message":"Password reset successful. Please sign in with your new password."})
}
