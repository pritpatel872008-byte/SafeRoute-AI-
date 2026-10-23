package main

import (
	"crypto/rand"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const userIDKey contextKey = "user_id"

type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *server) requireDB(w http.ResponseWriter) bool {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database_not_configured"})
		return false
	}
	return true
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) { return }
	var input credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"}); return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if len(input.Email) < 5 || len(input.Email) > 254 || !strings.Contains(input.Email, "@") || len(input.Password) < 12 || len(input.Password) > 72 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email_invalid_or_password_must_be_12_to_72_bytes"}); return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password_hash_failed"}); return }
	var id, email, name string
	err = s.db.QueryRow(r.Context(), `INSERT INTO app_users(email,password_hash,display_name) VALUES($1,$2,$3) RETURNING id,email,display_name`, input.Email, string(hash), input.DisplayName).Scan(&id,&email,&name)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account_creation_failed_email_may_already_exist"}); return
	}
	token, err := s.createSession(r, id)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session_creation_failed"}); return }
	writeJSON(w, http.StatusCreated, map[string]any{"user": map[string]string{"id":id,"email":email,"display_name":name},"token":token})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) { return }
	var input credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error":"invalid_json"}); return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	var id, email, name, hash string
	err := s.db.QueryRow(r.Context(), `SELECT id,email,display_name,password_hash FROM app_users WHERE email=$1`, input.Email).Scan(&id,&email,&name,&hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error":"invalid_credentials"}); return
	}
	token, err := s.createSession(r,id)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"session_creation_failed"}); return }
	writeJSON(w, http.StatusOK, map[string]any{"user":map[string]string{"id":id,"email":email,"display_name":name},"token":token})
}

func (s *server) createSession(r *http.Request, userID string) (string,error) {
	raw := make([]byte,32)
	if _,err := rand.Read(raw); err != nil { return "",err }
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	_,err := s.db.Exec(r.Context(), `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`,userID,hex.EncodeToString(sum[:]),time.Now().Add(7*24*time.Hour))
	return token,err
}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		if !s.requireDB(w) { return }
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header,"Bearer ") { writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"authentication_required"}); return }
		token := strings.TrimSpace(strings.TrimPrefix(header,"Bearer "))
		sum := sha256.Sum256([]byte(token))
		var userID string
		err := s.db.QueryRow(r.Context(), `SELECT user_id FROM sessions WHERE token_hash=$1 AND expires_at>now()`,hex.EncodeToString(sum[:])).Scan(&userID)
		if err != nil { writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"invalid_or_expired_session"}); return }
		ctx := context.WithValue(r.Context(),userIDKey,userID)
		next.ServeHTTP(w,r.WithContext(ctx))
	})
}

func (s *server) me(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userIDKey).(string)
	var email,name string
	if err := s.db.QueryRow(r.Context(),`SELECT email,display_name FROM app_users WHERE id=$1`,userID).Scan(&email,&name); err != nil {
		writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"account_not_found"}); return
	}
	writeJSON(w,http.StatusOK,map[string]any{"user":map[string]string{"id":userID,"email":email,"display_name":name}})
}

func (s *server) logout(w http.ResponseWriter,r *http.Request) {
	header := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer "))
	sum := sha256.Sum256([]byte(header))
	_,err := s.db.Exec(r.Context(),`DELETE FROM sessions WHERE token_hash=$1`,hex.EncodeToString(sum[:]))
	if err != nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"logout_failed"}); return }
	w.WriteHeader(http.StatusNoContent)
}

type contactInput struct {
	Name string `json:"name"`
	Phone string `json:"phone_e164"`
	Relationship string `json:"relationship"`
}

func (s *server) listContacts(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userIDKey).(string)
	rows,err := s.db.Query(r.Context(),`SELECT id,name,phone_e164,relationship,created_at FROM emergency_contacts WHERE user_id=$1 ORDER BY created_at`,userID)
	if err != nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"contacts_query_failed"}); return }
	defer rows.Close()
	contacts := make([]map[string]any,0)
	for rows.Next() {
		var id,name,phone,relationship string
		var created time.Time
		if err := rows.Scan(&id,&name,&phone,&relationship,&created); err != nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"contacts_read_failed"}); return }
		contacts=append(contacts,map[string]any{"id":id,"name":name,"phone_e164":phone,"relationship":relationship,"created_at":created})
	}
	if rows.Err()!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"contacts_read_failed"}); return }
	writeJSON(w,http.StatusOK,map[string]any{"contacts":contacts})
}

func (s *server) createContact(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userIDKey).(string)
	var input contactInput
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,8<<10)).Decode(&input); err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"}); return }
	input.Name=strings.TrimSpace(input.Name); input.Phone=strings.TrimSpace(input.Phone); input.Relationship=strings.TrimSpace(input.Relationship)
	if input.Name=="" || len(input.Name)>120 || len(input.Phone)<8 || len(input.Phone)>20 || !strings.HasPrefix(input.Phone,"+") {
		writeJSON(w,http.StatusBadRequest,map[string]string{"error":"name_and_e164_phone_required"}); return
	}
	var id string
	err:=s.db.QueryRow(r.Context(),`INSERT INTO emergency_contacts(user_id,name,phone_e164,relationship) VALUES($1,$2,$3,$4) RETURNING id`,userID,input.Name,input.Phone,input.Relationship).Scan(&id)
	if err!=nil { writeJSON(w,http.StatusConflict,map[string]string{"error":"contact_create_failed_phone_may_already_exist"}); return }
	writeJSON(w,http.StatusCreated,map[string]any{"contact":map[string]string{"id":id,"name":input.Name,"phone_e164":input.Phone,"relationship":input.Relationship}})
}

func (s *server) deleteContact(w http.ResponseWriter,r *http.Request) {
	userID := r.Context().Value(userIDKey).(string)
	id:=r.PathValue("id")
	tag,err:=s.db.Exec(r.Context(),`DELETE FROM emergency_contacts WHERE id=$1 AND user_id=$2`,id,userID)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"contact_delete_failed"}); return }
	if tag.RowsAffected()==0 { writeJSON(w,http.StatusNotFound,map[string]string{"error":"contact_not_found"}); return }
	w.WriteHeader(http.StatusNoContent)
}

var _ = pgx.ErrNoRows
