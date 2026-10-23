import { useCallback, useEffect, useRef, useState } from 'react';
import L from 'leaflet';
import { Activity, ArrowDownUp, Bell, CarFront, Compass, Crosshair, LocateFixed, MapPin, Menu, Navigation, ShieldCheck, Siren, Truck, Bike, Footprints, Users, X, LogIn, LogOut, UserPlus, Route as RouteIcon, Bookmark, Trash2, Clock, Search, CheckCircle2 } from 'lucide-react';

type Vehicle = 'car' | 'motorcycle' | 'truck' | 'walking';
type ApiStatus = { service?: string; database?: string };
type User = { id: string; email: string; display_name: string };
type PlaceOption = { label: string; latitude: number; longitude: number };
type RouteResult = { provider: string; profile: string; vehicle: Vehicle; distance_m: number; duration_s: number; geometry: { type: string; coordinates: [number, number][] }; steps: { instruction: string; distance_m: number; duration_s: number; location: number[] }[]; warnings: string[] };
type Contact = { id: string; name: string; phone_e164: string; relationship: string };
type SavedPlace = { id: string; label: string; latitude: number; longitude: number };
type RouteHistory = { id: string; origin: PlaceOption; destination: PlaceOption; vehicle: Vehicle; route_summary: { distance_m?: number; duration_s?: number }; created_at: string };
type SOSEvent = { id: string; latitude: number; longitude: number; note: string; state: string; created_at: string };
const API = (import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const MAP_KEY = import.meta.env.VITE_MAPTILER_KEY || '';
const tokenKey = 'saferoute.session';

async function api<T>(path: string, options: RequestInit = {}, token = ''): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body) headers.set('Content-Type', 'application/json');
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const response = await fetch(`${API}${path}`, { ...options, headers });
  if (response.status === 204) return undefined as T;
  const body = await response.json().catch(() => ({})) as { error?: string; detail?: string } & T;
  if (!response.ok) throw new Error(body.detail || body.error || `Request failed (HTTP ${response.status})`);
  return body as T;
}
function distanceLabel(m: number) { return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`; }
function durationLabel(s: number) { const min = Math.round(s / 60); return min >= 60 ? `${Math.floor(min / 60)} hr ${min % 60} min` : `${min} min`; }

export default function App() {
  const [originText, setOriginText] = useState('');
  const [destinationText, setDestinationText] = useState('');
  const [origin, setOrigin] = useState<PlaceOption | null>(null);
  const [destination, setDestination] = useState<PlaceOption | null>(null);
  const [originOptions, setOriginOptions] = useState<PlaceOption[]>([]);
  const [destinationOptions, setDestinationOptions] = useState<PlaceOption[]>([]);
  const [vehicle, setVehicle] = useState<Vehicle>('car');
  const [apiStatus, setApiStatus] = useState<ApiStatus | null>(null);
  const [apiError, setApiError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [route, setRoute] = useState<RouteResult | null>(null);
  const [token, setToken] = useState(() => sessionStorage.getItem(tokenKey) || '');
  const [user, setUser] = useState<User | null>(null);
  const [authOpen, setAuthOpen] = useState(false);
  const [authMode, setAuthMode] = useState<'login' | 'register' | 'forgot' | 'reset'>('login');
  const [resetToken, setResetToken] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [authMessage, setAuthMessage] = useState('');
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [contactName, setContactName] = useState('');
  const [contactPhone, setContactPhone] = useState('');
  const [contactRelation, setContactRelation] = useState('');
  const [savedPlaces, setSavedPlaces] = useState<SavedPlace[]>([]);
  const [history, setHistory] = useState<RouteHistory[]>([]);
  const [sosEvents, setSosEvents] = useState<SOSEvent[]>([]);
  const [sosStage, setSosStage] = useState<'idle' | 'confirm' | 'ready' | 'sending'>('idle');
  const [sosNote, setSosNote] = useState('');
  const [sosMessage, setSosMessage] = useState('');
  const [sosCountdown, setSosCountdown] = useState(5);
  const mapElement = useRef<HTMLDivElement | null>(null);
  const mapInstance = useRef<L.Map | null>(null);
  const routeLayer = useRef<L.GeoJSON | null>(null);
  const originMarker = useRef<L.Marker | null>(null);
  const destinationMarker = useRef<L.Marker | null>(null);

  const refreshPrivateData = useCallback(async (currentToken: string) => {
    const [contactData, placeData, historyData, sosData] = await Promise.all([
      api<{ contacts: Contact[] }>('/api/v1/contacts', {}, currentToken),
      api<{ places: SavedPlace[] }>('/api/v1/places', {}, currentToken),
      api<{ routes: RouteHistory[] }>('/api/v1/routes/history', {}, currentToken),
      api<{ events: SOSEvent[] }>('/api/v1/sos', {}, currentToken)
    ]);
    setContacts(contactData.contacts || []);
    setSavedPlaces(placeData.places || []);
    setHistory(historyData.routes || []);
    setSosEvents(sosData.events || []);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const reset = params.get('token');
    const resetEmail = params.get('email');
    if (reset && resetEmail) {
      setResetToken(reset);
      setEmail(resetEmail);
      setAuthMode('reset');
      setAuthOpen(true);
      window.history.replaceState({}, '', window.location.pathname + window.location.hash);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    api<ApiStatus>('/api/v1/status', { signal: controller.signal }).then(setApiStatus).catch(error => {
      if (!(error instanceof Error) || error.name !== 'AbortError') setApiError('API unavailable. Configure and start the backend to enable connected services.');
    });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!token) { setUser(null); setContacts([]); setSavedPlaces([]); setHistory([]); setSosEvents([]); return; }
    let cancelled = false;
    api<{ user: User }>('/api/v1/auth/me', {}, token).then(async result => {
      if (cancelled) return;
      setUser(result.user);
      try { await refreshPrivateData(token); } catch (error) { setMessage(error instanceof Error ? error.message : 'Could not load account data.'); }
    }).catch(() => {
      if (!cancelled) { sessionStorage.removeItem(tokenKey); setToken(''); setUser(null); }
    });
    return () => { cancelled = true; };
  }, [token, refreshPrivateData]);

  useEffect(() => {
    if (!mapElement.current || !MAP_KEY) return;
    const map = L.map(mapElement.current, { zoomControl: true }).setView([20.5937, 78.9629], 5);
    mapInstance.current = map;
    L.tileLayer(`https://api.maptiler.com/maps/streets-v2/{z}/{x}/{y}.png?key=${encodeURIComponent(MAP_KEY)}`, {
      tileSize: 512, zoomOffset: -1, minZoom: 1, attribution: '&copy; MapTiler &copy; OpenStreetMap contributors'
    }).addTo(map);
    return () => { map.remove(); mapInstance.current = null; };
  }, []);

  useEffect(() => {
    if (!mapInstance.current || !route) return;
    routeLayer.current?.remove();
    originMarker.current?.remove();
    destinationMarker.current?.remove();
    const layer = L.geoJSON(route.geometry as GeoJSON.GeoJsonObject, { style: { color: '#927cff', weight: 5, opacity: .92 } }).addTo(mapInstance.current);
    routeLayer.current = layer;
    const coords = route.geometry.coordinates;
    if (coords.length) {
      originMarker.current = L.marker([coords[0][1], coords[0][0]]).addTo(mapInstance.current).bindPopup('Start');
      destinationMarker.current = L.marker([coords[coords.length - 1][1], coords[coords.length - 1][0]]).addTo(mapInstance.current).bindPopup('Destination');
      mapInstance.current.fitBounds(layer.getBounds(), { padding: [35, 35], maxZoom: 15 });
      window.setTimeout(() => mapInstance.current?.invalidateSize(), 100);
    }
  }, [route]);

  useEffect(() => {
    if (sosStage !== 'confirm') return;
    setSosCountdown(5);
    const endAt = Date.now() + 5000;
    const interval = window.setInterval(() => {
      const left = Math.max(0, Math.ceil((endAt - Date.now()) / 1000));
      setSosCountdown(left);
      if (left <= 0) { window.clearInterval(interval); setSosStage('ready'); }
    }, 150);
    return () => window.clearInterval(interval);
  }, [sosStage]);

  useEffect(() => {
    const query = originText.trim();
    if (query.length < 3 || origin?.label === query) { setOriginOptions([]); return; }
    const timer = window.setTimeout(() => {
      api<{ features: PlaceOption[] }>(`/api/v1/geocode?q=${encodeURIComponent(query)}`).then(x => setOriginOptions(x.features || [])).catch(() => setOriginOptions([]));
    }, 350);
    return () => window.clearTimeout(timer);
  }, [originText, origin]);

  useEffect(() => {
    const query = destinationText.trim();
    if (query.length < 3 || destination?.label === query) { setDestinationOptions([]); return; }
    const timer = window.setTimeout(() => {
      api<{ features: PlaceOption[] }>(`/api/v1/geocode?q=${encodeURIComponent(query)}`).then(x => setDestinationOptions(x.features || [])).catch(() => setDestinationOptions([]));
    }, 350);
    return () => window.clearTimeout(timer);
  }, [destinationText, destination]);

  function chooseOrigin(place: PlaceOption) { setOrigin(place); setOriginText(place.label); setOriginOptions([]); }
  function chooseDestination(place: PlaceOption) { setDestination(place); setDestinationText(place.label); setDestinationOptions([]); }

  async function useCurrentLocation() {
    setMessage('');
    if (!navigator.geolocation) { setMessage('This browser does not provide geolocation.'); return; }
    setBusy(true);
    navigator.geolocation.getCurrentPosition(position => {
      const place = { label: 'Current location', latitude: position.coords.latitude, longitude: position.coords.longitude };
      setOrigin(place); setOriginText(`${place.latitude.toFixed(5)}, ${place.longitude.toFixed(5)}`); setOriginOptions([]); setBusy(false);
    }, error => {
      setMessage(error.code === error.PERMISSION_DENIED ? 'Location permission was denied.' : 'Could not obtain location. Check device location settings and try again.');
      setBusy(false);
    }, { enableHighAccuracy: true, timeout: 12000, maximumAge: 10000 });
  }

  async function planRoute(event: React.FormEvent) {
    event.preventDefault(); setMessage(''); setRoute(null);
    if (!origin || !destination) { setMessage('Choose both locations from the real geocoding suggestions before requesting a route.'); return; }
    setBusy(true);
    try {
      const result = await api<RouteResult>('/api/v1/routes', { method: 'POST', body: JSON.stringify({ origin: `${origin.latitude},${origin.longitude}`, destination: `${destination.latitude},${destination.longitude}`, vehicle }) });
      setRoute(result); setMessage('Route received from MapTiler. Check local signs and restrictions before travelling.');
      if (token) {
        try { await api('/api/v1/routes/history', { method: 'POST', body: JSON.stringify({ origin, destination, vehicle, provider: result.provider, route_summary: { distance_m: result.distance_m, duration_s: result.duration_s, profile: result.profile } }) }, token); await refreshPrivateData(token); }
        catch (e) { setMessage(`Route calculated, but history could not be saved: ${e instanceof Error ? e.message : 'unknown error'}`); }
      }
    } catch (e) { setMessage(e instanceof Error ? e.message : 'Route calculation failed.'); }
    finally { setBusy(false); }
  }

  async function submitAuth(event: React.FormEvent) {
    event.preventDefault(); setAuthMessage(''); setBusy(true);
    try {
      if (authMode === 'forgot') {
        const result = await api<{ message: string }>('/api/v1/auth/password-reset/request', {
          method: 'POST', body: JSON.stringify({ email })
        });
        setAuthMessage(result.message || 'If an account exists for that address, a reset email will be sent.');
        return;
      }
      if (authMode === 'reset') {
        const result = await api<{ message: string }>('/api/v1/auth/password-reset/confirm', {
          method: 'POST', body: JSON.stringify({ email, token: resetToken, new_password: password })
        });
        setPassword('');
        setResetToken('');
        setAuthMode('login');
        setAuthMessage(result.message || 'Password reset successful. Please sign in.');
        return;
      }
      const result = await api<{ token: string; user: User }>(authMode === 'register' ? '/api/v1/auth/register' : '/api/v1/auth/login', {
        method: 'POST', body: JSON.stringify({ email, password, display_name: displayName })
      });
      sessionStorage.setItem(tokenKey, result.token); setToken(result.token); setUser(result.user); setAuthOpen(false); setPassword(''); setAuthMessage('');
    } catch (e) { setAuthMessage(e instanceof Error ? e.message : 'Authentication failed.'); }
    finally { setBusy(false); }
  }

  async function logout() {
    const oldToken = token; sessionStorage.removeItem(tokenKey); setToken(''); setUser(null); setContacts([]); setSavedPlaces([]); setHistory([]);
    try { await api('/api/v1/auth/logout', { method: 'POST' }, oldToken); } catch { /* Local session is cleared even if API is offline. */ }
  }

  async function addContact(event: React.FormEvent) {
    event.preventDefault(); if (!token) { setAuthOpen(true); return; } setBusy(true);
    try { await api('/api/v1/contacts', { method: 'POST', body: JSON.stringify({ name: contactName, phone_e164: contactPhone, relationship: contactRelation }) }, token); setContactName(''); setContactPhone(''); setContactRelation(''); await refreshPrivateData(token); }
    catch (e) { setMessage(e instanceof Error ? e.message : 'Could not add contact.'); }
    finally { setBusy(false); }
  }
  async function deleteContact(id: string) {
    if (!token) return; try { await api(`/api/v1/contacts/${encodeURIComponent(id)}`, { method: 'DELETE' }, token); await refreshPrivateData(token); }
    catch (e) { setMessage(e instanceof Error ? e.message : 'Could not delete contact.'); }
  }
  async function savePlace(place: PlaceOption) {
    if (!token) { setAuthOpen(true); setAuthMessage('Sign in to save places.'); return; }
    try { await api('/api/v1/places', { method: 'POST', body: JSON.stringify({ label: place.label, latitude: place.latitude, longitude: place.longitude }) }, token); await refreshPrivateData(token); }
    catch (e) { setMessage(e instanceof Error ? e.message : 'Could not save place.'); }
  }
  async function deletePlace(id: string) {
    if (!token) return; try { await api(`/api/v1/places/${encodeURIComponent(id)}`, { method: 'DELETE' }, token); await refreshPrivateData(token); }
    catch (e) { setMessage(e instanceof Error ? e.message : 'Could not delete saved place.'); }
  }

  async function sendSOS() {
    if (!token) { setAuthOpen(true); setAuthMessage('Sign in before creating a persisted SOS event.'); return; }
    setSosStage('sending'); setSosMessage('');
    if (!navigator.geolocation) { setSosMessage('This browser does not provide GPS location. No SOS event was created.'); setSosStage('idle'); return; }
    navigator.geolocation.getCurrentPosition(async position => {
      try {
        const result = await api<{ event: { id: string; state: string; delivery: string } }>('/api/v1/sos', { method: 'POST', body: JSON.stringify({ latitude: position.coords.latitude, longitude: position.coords.longitude, note: sosNote }) }, token);
        setSosMessage(`SOS event ${result.event.id} was recorded in the database. Emergency contacts were NOT notified because no delivery provider is configured.`);
        setSosStage('idle'); setSosNote('');
      } catch (e) { setSosMessage(e instanceof Error ? e.message : 'SOS event could not be recorded.'); setSosStage('idle'); }
    }, error => { setSosMessage(error.code === error.PERMISSION_DENIED ? 'Location permission denied. No SOS event was created.' : 'Could not obtain GPS. No SOS event was created.'); setSosStage('idle'); }, { enableHighAccuracy: true, timeout: 12000, maximumAge: 0 });
  }

  const database = apiStatus?.database ?? (apiError ? 'unavailable' : 'checking');
  const suggestions = (items: PlaceOption[], onChoose: (p: PlaceOption) => void) => items.length > 0 && <div className="suggestions" role="listbox">{items.map((p, i) => <button type="button" role="option" key={`${p.label}-${i}`} onClick={() => onChoose(p)}><MapPin size={15} /><span>{p.label}<small>{p.latitude.toFixed(4)}, {p.longitude.toFixed(4)}</small></span></button>)}</div>;

  return <div className="app-shell">
    <header className="topbar">
      <a className="brand" href="#top" aria-label="SafeRoute AI home"><span className="brand-mark"><Navigation size={21} /></span><span>SafeRoute<span className="brand-accent">AI</span><small>SAFETY-FIRST MOBILITY</small></span></a>
      <nav className={mobileMenu ? 'nav-links open' : 'nav-links'} aria-label="Main navigation"><a className="active" href="#navigate" onClick={() => setMobileMenu(false)}>Navigate</a><a href="#safety" onClick={() => setMobileMenu(false)}>Safety center</a><a href="#account" onClick={() => setMobileMenu(false)}>My places</a><a href="#status" onClick={() => setMobileMenu(false)}>System status</a></nav>
      <div className="topbar-right">{user ? <><span className="connection"><span className="status-dot live" />{user.display_name || user.email}</span><button className="account-button" onClick={logout}><LogOut size={15} /> Sign out</button></> : <button className="account-button" onClick={() => { setAuthOpen(true); setAuthMode('login'); }}><LogIn size={15} /> Sign in</button>}<button className="icon-button mobile-toggle" onClick={() => setMobileMenu(!mobileMenu)} aria-label="Toggle menu">{mobileMenu ? <X size={20} /> : <Menu size={20} />}</button></div>
    </header>

    <main id="top">
      <section className="hero"><div className="hero-copy"><div className="eyebrow"><ShieldCheck size={15} /> SAFER ROUTES. CLEARER DECISIONS.</div><h1>Move with<br /><span>confidence.</span></h1><p>Plan your journey, review the live map and keep your emergency tools within reach. Route and connection details are shown only when returned by connected services.</p><div className="hero-stats"><div><span className="stat-icon"><Compass size={17} /></span><span><strong>Route planning</strong><small>{MAP_KEY ? 'Map tiles configured' : 'Map provider setup required'}</small></span></div><div><span className="stat-icon"><Siren size={17} /></span><span><strong>SOS workflow</strong><small>GPS + explicit confirmation</small></span></div></div></div><div className="hero-status panel"><div className="hero-status-top"><span className="eyebrow">CONNECTION CHECK</span><span className={apiStatus ? "status-dot live" : "status-dot"} /></div><div className="hero-status-row"><span className="status-symbol"><Activity size={17}/></span><span><strong>Routing API</strong><small>{apiStatus ? "Responding" : apiError ? "Unavailable" : "Checking connection"}</small></span></div><div className="hero-status-row"><span className="status-symbol"><ShieldCheck size={17}/></span><span><strong>Database</strong><small>{database === "connected" ? "Connected" : database === "not_configured" ? "Not configured" : database === "unavailable" ? "Unavailable" : "Checking connection"}</small></span></div><div className="hero-status-note">Live service status · no simulated readings</div></div></section>

      <section className="workspace" id="navigate"><div className="section-heading"><div><span className="eyebrow">ROUTE WORKSPACE</span><h2>Where are you going?</h2><p>Search a starting point and destination, then calculate a route using the connected provider.</p></div><span className="secure-label"><ShieldCheck size={15} /> Provider-backed</span></div>
        <div className="planner-grid"><form className="panel route-panel" onSubmit={planRoute}><div className="panel-heading"><div className="panel-icon"><Compass size={19} /></div><div><h3>Route planner</h3><p>Origin to destination</p></div></div>
          <label className="field-label" htmlFor="origin">STARTING POINT</label><div className="location-input"><span className="field-pin start"><MapPin size={17} /></span><input id="origin" value={originText} onChange={e => { setOriginText(e.target.value); setOrigin(null); }} placeholder="Search a real address or place" autoComplete="off" /><button type="button" className="field-action" aria-label="Use current location" onClick={useCurrentLocation} disabled={busy}><LocateFixed size={17} /></button></div>{suggestions(originOptions, chooseOrigin)}{origin && <button type="button" className="selected-location" onClick={() => savePlace(origin)}><Bookmark size={13} /> Save starting point</button>}
          <div className="field-connector" /><label className="field-label" htmlFor="destination">DESTINATION</label><div className="location-input"><span className="field-pin end"><MapPin size={17} /></span><input id="destination" value={destinationText} onChange={e => { setDestinationText(e.target.value); setDestination(null); }} placeholder="Where do you want to go?" autoComplete="off" /></div>{suggestions(destinationOptions, chooseDestination)}{destination && <button type="button" className="selected-location" onClick={() => savePlace(destination)}><Bookmark size={13} /> Save destination</button>}
          <div className="vehicle-label">TRAVEL MODE</div><div className="vehicle-options">{([{id:'car',label:'Car',icon:<CarFront size={18} />},{id:'motorcycle',label:'Two-wheeler',icon:<Bike size={18} />},{id:'truck',label:'Truck',icon:<Truck size={18} />},{id:'walking',label:'Walking',icon:<Footprints size={18} />}] as const).map(item => <button type="button" key={item.id} className={vehicle === item.id ? 'vehicle-option selected' : 'vehicle-option'} onClick={() => setVehicle(item.id)} aria-pressed={vehicle === item.id}>{item.icon}<span>{item.label}</span></button>)}</div>
          <button className="primary-button" type="submit" disabled={busy}><Navigation size={18} /> {busy ? 'Working…' : 'Calculate route'} <ArrowDownUp size={17} /></button>{message && <p className="inline-message" role="status">{message}</p>}<p className="fine-print">Truck-specific restrictions are not available from this configuration. Route data can miss closures; obey local signs and road rules.</p>
        </form>
        <div className="panel map-panel"><div className="map-topline"><div><span className="eyebrow">INTERACTIVE NAVIGATION</span><h3>Live journey map</h3></div><span className="map-chip"><Activity size={14} /> {MAP_KEY ? 'Map tiles configured' : 'Map key pending'}</span></div><div className={MAP_KEY ? 'map-placeholder real-map' : 'map-placeholder'}>{MAP_KEY ? <div className="leaflet-map" ref={mapElement} aria-label="Interactive live map" /> : <div className="map-center"><span className="map-center-icon"><Compass size={24} /></span><strong>Map provider not configured</strong><span>Set VITE_MAPTILER_KEY for tiles and MAPTILER_API_KEY on the API for geocoding and directions.</span></div>}</div>{route && <div className="route-summary"><div><small>ESTIMATED DISTANCE</small><strong>{distanceLabel(route.distance_m)}</strong></div><div><small>ESTIMATED TIME</small><strong>{durationLabel(route.duration_s)}</strong></div><span className="feature-state"><CheckCircle2 size={14} /> {route.provider}</span></div>}<div className="map-footer"><span><span className={database === 'connected' ? 'status-dot live' : 'status-dot'} /> {database === 'connected' ? 'Database connected' : database === 'not_configured' ? 'Database not configured' : database === 'unavailable' ? 'API unavailable' : 'Checking connection'}</span><span>Mode: {vehicle === 'motorcycle' ? 'Two-wheeler' : vehicle[0].toUpperCase()+vehicle.slice(1)}</span></div>{route?.steps?.length ? <details className="turn-list"><summary><RouteIcon size={15} /> Turn-by-turn instructions ({route.steps.length})</summary>{route.steps.map((step,i)=><div key={i} className="turn-step"><span>{i+1}</span><p>{step.instruction || 'Continue on route'}<small>{distanceLabel(step.distance_m)} · {durationLabel(step.duration_s)}</small></p></div>)}</details> : null}</div></div>
      </section>

      <section className="safety-section" id="safety"><div className="section-heading"><div><span className="eyebrow">PREPAREDNESS</span><h2>Your safety center</h2><p>SOS events are recorded only after explicit confirmation and successful GPS acquisition.</p></div></div><div className="safety-grid"><article className="panel safety-card"><div className="safety-card-icon"><Siren size={21} /></div><h3>Emergency SOS</h3><p>Creates a persistent GPS-backed SOS event after a five-second confirmation step. It does not contact emergency services unless a delivery provider is separately configured.</p>{sosStage==='idle' ? <button className="danger-button" onClick={() => { if (!token) { setAuthOpen(true); setAuthMessage('Sign in to create an SOS event.'); return; } setSosStage('confirm'); setSosMessage(''); }}> <Siren size={16} /> Start SOS workflow</button> : sosStage==='confirm' ? <div className="sos-confirm"><strong>Confirm emergency action</strong><p>Only continue if you intend to create an SOS event. You can cancel now.</p><label className="field-label" htmlFor="sos-note">OPTIONAL NOTE</label><textarea id="sos-note" value={sosNote} onChange={e=>setSosNote(e.target.value)} maxLength={500} placeholder="Helpful context for the event record" /><div className="sos-actions"><button className="danger-button" disabled><Clock size={15} /> Wait {sosCountdown}s</button><button className="secondary-button" onClick={()=>setSosStage('idle')}>Cancel</button></div></div> : sosStage==='ready' ? <div className="sos-confirm"><strong>Ready for final confirmation</strong><p>GPS will be requested now. A database record is not a notification to contacts.</p><button className="danger-button" onClick={sendSOS}><Siren size={16} /> Confirm and record SOS</button><button className="secondary-button" onClick={()=>setSosStage('idle')}>Cancel</button></div> : <p>Requesting GPS and recording the event…</p>}{sosMessage && <p className="inline-message" role="status">{sosMessage}</p>}{user && <div className="contact-list"><strong className="list-heading">Recent SOS events</strong>{sosEvents.length ? sosEvents.slice(0,5).map(event=><div className="data-row" key={event.id}><span><strong>{event.state.replace('_',' ')}</strong><small>{new Date(event.created_at).toLocaleString()} · {event.latitude.toFixed(4)}, {event.longitude.toFixed(4)}</small></span></div>) : <p className="empty-state">No SOS events recorded.</p>}</div>}</article>
        <article className="panel safety-card"><div className="safety-card-icon"><Users size={21} /></div><h3>Emergency contacts</h3><p>Add and manage your own contacts. Phone numbers must include the international + country code.</p>{user ? <><form className="compact-form" onSubmit={addContact}><input required value={contactName} onChange={e=>setContactName(e.target.value)} placeholder="Contact name" maxLength={120}/><input required value={contactPhone} onChange={e=>setContactPhone(e.target.value)} placeholder="+919876543210" minLength={8} maxLength={20}/><input value={contactRelation} onChange={e=>setContactRelation(e.target.value)} placeholder="Relationship (optional)" maxLength={80}/><button className="secondary-button" type="submit" disabled={busy}><Users size={15}/> Add contact</button></form><div className="contact-list">{contacts.map(c=><div key={c.id} className="data-row"><span><strong>{c.name}</strong><small>{c.phone_e164}{c.relationship ? ` · ${c.relationship}` : ''}</small></span><button className="subtle-icon" onClick={()=>deleteContact(c.id)} aria-label={`Delete ${c.name}`}><Trash2 size={15}/></button></div>)}{!contacts.length&&<p className="empty-state">No emergency contacts saved yet.</p>}</div></> : <button className="secondary-button" onClick={()=>{setAuthOpen(true);setAuthMode('login');}}><LogIn size={15}/> Sign in to manage contacts</button>}</article>
        <article className="panel safety-card"><div className="safety-card-icon"><ShieldCheck size={21} /></div><h3>Road awareness</h3><p>MapTiler route data is used for directions when configured. This build does not claim verified live closures, official road classifications or truck restrictions.</p><span className="feature-state"><span className="status-dot" /> Road restriction feed not configured</span></article></div></section>

      <section className="workspace" id="account"><div className="section-heading"><div><span className="eyebrow">PERSONAL WORKSPACE</span><h2>Saved places & recent routes</h2><p>Your private data is loaded from the API after sign-in.</p></div>{user ? <span className="secure-label"><ShieldCheck size={15}/> {user.email}</span> : <button className="secondary-button" onClick={()=>setAuthOpen(true)}><LogIn size={15}/> Sign in</button>}</div><div className="account-grid"><div className="panel data-panel"><div className="panel-heading"><div className="panel-icon"><Bookmark size={18}/></div><div><h3>Saved places</h3><p>Stored in your account</p></div></div>{!user ? <p className="empty-state">Sign in to save and retrieve places.</p> : savedPlaces.length ? savedPlaces.map(p=><div className="data-row" key={p.id}><button className="place-link" onClick={()=>{const loc={label:p.label,latitude:p.latitude,longitude:p.longitude};setDestination(loc);setDestinationText(loc.label);setDestinationOptions([]);}}><MapPin size={15}/><span><strong>{p.label}</strong><small>{p.latitude.toFixed(4)}, {p.longitude.toFixed(4)}</small></span></button><button className="subtle-icon" onClick={()=>deletePlace(p.id)} aria-label={`Delete ${p.label}`}><Trash2 size={15}/></button></div>) : <p className="empty-state">No saved places yet. Choose a geocoded location above and tap Save.</p>}</div><div className="panel data-panel"><div className="panel-heading"><div className="panel-icon"><Clock size={18}/></div><div><h3>Route history</h3><p>Latest 50 saved routes</p></div></div>{!user ? <p className="empty-state">Sign in to persist your calculated route history.</p> : history.length ? history.map(h=><div className="data-row" key={h.id}><span><strong>{h.origin?.label || 'Origin'} → {h.destination?.label || 'Destination'}</strong><small>{h.vehicle} · {h.route_summary?.distance_m ? distanceLabel(h.route_summary.distance_m) : 'Distance unavailable'} · {new Date(h.created_at).toLocaleString()}</small></span><button className="subtle-icon" title="Use destination" onClick={()=>{setOrigin(h.origin);setOriginText(h.origin?.label||'');setDestination(h.destination);setDestinationText(h.destination?.label||'');setVehicle(h.vehicle);}}><Navigation size={15}/></button></div>) : <p className="empty-state">No routes saved yet. Sign in and calculate a route to keep its summary.</p>}</div></div></section>

      <section className="system-section" id="status"><div className="section-heading"><div><span className="eyebrow">TRANSPARENCY</span><h2>System status</h2><p>Values below come from real API responses and current configuration.</p></div><button className="secondary-button" onClick={()=>{setApiStatus(null);setApiError('');api<ApiStatus>('/api/v1/status').then(setApiStatus).catch(e=>setApiError(e instanceof Error?e.message:'API unavailable'));}}><Activity size={15}/> Refresh status</button></div><div className="panel status-panel"><div className="status-row"><span className="status-symbol"><Activity size={18}/></span><span><strong>SafeRoute API</strong><small>{apiError || (apiStatus ? 'API responded successfully.' : 'Checking configured API endpoint…')}</small></span><span className={apiStatus?'status-value good':'status-value'}>{apiStatus?'Connected':'Unavailable / pending'}</span></div><div className="status-row"><span className="status-symbol"><ShieldCheck size={18}/></span><span><strong>PostgreSQL</strong><small>{database==='connected'?'Database ping succeeded.':database==='not_configured'?'DATABASE_URL is not set on the API.':database==='unavailable'?'API could not confirm database connectivity.':'Awaiting API response.'}</small></span><span className={database==='connected'?'status-value good':'status-value'}>{database}</span></div><div className="status-row"><span className="status-symbol"><MapPin size={18}/></span><span><strong>Geocoding & directions</strong><small>{MAP_KEY?'Web map tiles configured; backend MAPTILER_API_KEY is required for search and routes.':'Configure VITE_MAPTILER_KEY and API MAPTILER_API_KEY.'}</small></span><span className={MAP_KEY?'status-value good':'status-value'}>{MAP_KEY?'Partially configured':'Not configured'}</span></div><div className="status-row"><span className="status-symbol"><Bell size={18}/></span><span><strong>SOS delivery</strong><small>GPS events can be recorded. No SMS/voice notification provider is configured.</small></span><span className="status-value">Not configured</span></div></div></section>
    </main>
    <footer><a className="brand footer-brand" href="#top"><span className="brand-mark"><Navigation size={18}/></span><span>SafeRoute<span className="brand-accent">AI</span><small>BUILT FOR MORE CONFIDENT JOURNEYS</small></span></a><p>Navigation assistance only — not a substitute for emergency services or posted road instructions.</p><span>SafeRoute AI • Provider-backed features report their real status</span></footer>

    {authOpen && <div className="modal-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget){setAuthOpen(false);setAuthMessage('');}}}><section className="auth-modal panel" role="dialog" aria-modal="true" aria-labelledby="auth-title"><button className="subtle-icon modal-close" onClick={()=>{setAuthOpen(false);setAuthMessage('');}} aria-label="Close sign-in dialog"><X size={18}/></button><div className="panel-icon"><ShieldCheck size={21}/></div><h2 id="auth-title">{authMode==='login'?'Welcome back':authMode==='register'?'Create your SafeRoute account':authMode==='forgot'?'Reset your password':'Choose a new password'}</h2><p>{authMode==='login'?'Sign in to access contacts, saved places and route history.':authMode==='register'?'Create an account with a password of at least 12 characters.':authMode==='forgot'?'We will send a one-time reset link if email delivery is configured and an account exists.':'Your reset link is one-time and expires after 30 minutes.'}</p><form onSubmit={submitAuth} className="auth-form">{authMode==='register'&&<label>Display name<input required value={displayName} onChange={e=>setDisplayName(e.target.value)} maxLength={100} autoComplete="name"/></label>}<label>Email<input type="email" required value={email} onChange={e=>setEmail(e.target.value)} autoComplete="email" maxLength={254}/></label>{(authMode==='login'||authMode==='register'||authMode==='reset')&&<label>{authMode==='reset'?'New password':'Password'}<input type="password" required value={password} onChange={e=>setPassword(e.target.value)} minLength={12} maxLength={72} autoComplete={authMode==='login'?'current-password':'new-password'}/></label>}{authMode==='reset'&&<p className="fine-print">Reset token loaded from the email link. If it is expired or invalid, request another reset link.</p>}<button className="primary-button" disabled={busy} type="submit">{authMode==='login'?<LogIn size={17}/>:authMode==='register'?<UserPlus size={17}/>:<ShieldCheck size={17}/>} {busy?'Please wait…':authMode==='login'?'Sign in':authMode==='register'?'Create account':authMode==='forgot'?'Send reset link':'Set new password'}</button>{authMessage&&<p className="inline-message" role="alert">{authMessage}</p>}</form>{authMode==='login'&&<p className="auth-switch"><button onClick={()=>{setAuthMode('forgot');setAuthMessage('');}}>Forgot password?</button></p>}{(authMode==='login'||authMode==='register')&&<p className="auth-switch">{authMode==='login'?'New to SafeRoute?':'Already have an account?'} <button onClick={()=>{setAuthMode(authMode==='login'?'register':'login');setAuthMessage('');}}>{authMode==='login'?'Create account':'Sign in'}</button></p>}{(authMode==='forgot'||authMode==='reset')&&<p className="auth-switch"><button onClick={()=>{setAuthMode('login');setPassword('');setResetToken('');setAuthMessage('');}}>Back to sign in</button></p>}<small className="fine-print">Sessions expire after seven days. Reset emails require a verified sender and configured provider credentials.</small></section></div>}
  </div>;
}
