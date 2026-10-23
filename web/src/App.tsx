import { useEffect, useRef, useState } from 'react';
import L from 'leaflet';
import { Activity, ArrowDownUp, Bell, CarFront, Compass, Crosshair, LocateFixed, MapPin, Menu, Navigation, ShieldCheck, Siren, Truck, Bike, Footprints, Users, X } from 'lucide-react';

type Vehicle = 'car' | 'motorcycle' | 'truck' | 'walking';
type ApiStatus = { service?: string; database?: string };

const API = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export default function App() {
  const [origin, setOrigin] = useState('');
  const [destination, setDestination] = useState('');
  const [vehicle, setVehicle] = useState<Vehicle>('car');
  const [apiStatus, setApiStatus] = useState<ApiStatus | null>(null);
  const [apiError, setApiError] = useState('');
  const [locationMessage, setLocationMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const mapElement = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    fetch(`${API}/api/v1/status`, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(`API returned HTTP ${response.status}`);
        return response.json() as Promise<ApiStatus>;
      })
      .then(setApiStatus)
      .catch(error => {
        if (error.name !== 'AbortError') setApiError('API unavailable. Start the backend to enable connected services.');
      });
    return () => controller.abort();
  }, []);

  async function useCurrentLocation() {
    setLocationMessage('');
    if (!navigator.geolocation) {
      setLocationMessage('This browser does not provide geolocation.');
      return;
    }
    setBusy(true);
    navigator.geolocation.getCurrentPosition(
      position => {
        setOrigin(`${position.coords.latitude.toFixed(5)}, ${position.coords.longitude.toFixed(5)}`);
        setLocationMessage('Current coordinates captured in this browser.');
        setBusy(false);
      },
      error => {
        setLocationMessage(error.code === error.PERMISSION_DENIED ? 'Location permission was denied.' : 'Could not obtain your location. Check device location settings and try again.');
        setBusy(false);
      },
      { enableHighAccuracy: true, timeout: 12000, maximumAge: 10000 }
    );
  }

  function planRoute(event: React.FormEvent) {
    event.preventDefault();
    setLocationMessage('');
    if (!origin.trim() || !destination.trim()) {
      setLocationMessage('Enter both an origin and destination before planning a route.');
      return;
    }
    setLocationMessage('Route provider integration is not configured yet. Add a MapTiler key and routing endpoint before route results can be shown.');
  }

  useEffect(() => {
    if (!mapElement.current) return;
    const key = import.meta.env.VITE_MAPTILER_KEY;
    if (!key) return;
    const map = L.map(mapElement.current, { zoomControl: true }).setView([20.5937, 78.9629], 5);
    L.tileLayer(`https://api.maptiler.com/maps/streets-v2/{z}/{x}/{y}.png?key=${encodeURIComponent(key)}`, {
      tileSize: 512, zoomOffset: -1, minZoom: 1, attribution: '&copy; MapTiler &copy; OpenStreetMap contributors'
    }).addTo(map);
    return () => map.remove();
  }, []);

  const database = apiStatus?.database ?? (apiError ? 'unavailable' : 'checking');

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="#" aria-label="SafeRoute AI home"><span className="brand-mark"><Navigation size={21} /></span><span>SafeRoute<span className="brand-accent">AI</span><small>SAFETY-FIRST MOBILITY</small></span></a>
        <nav className={mobileMenu ? 'nav-links open' : 'nav-links'} aria-label="Main navigation">
          <a className="active" href="#navigate" onClick={() => setMobileMenu(false)}>Navigate</a>
          <a href="#safety" onClick={() => setMobileMenu(false)}>Safety center</a>
          <a href="#status" onClick={() => setMobileMenu(false)}>System status</a>
        </nav>
        <div className="topbar-right"><span className="connection"><span className={apiStatus ? 'status-dot live' : 'status-dot'} />{apiStatus ? 'API connected' : 'API not connected'}</span><button className="icon-button mobile-toggle" onClick={() => setMobileMenu(!mobileMenu)} aria-label="Toggle menu">{mobileMenu ? <X size={20} /> : <Menu size={20} />}</button></div>
      </header>

      <main>
        <section className="hero">
          <div className="hero-copy"><div className="eyebrow"><ShieldCheck size={15} /> INTELLIGENT MOBILITY • PERSONAL SAFETY</div><h1>Every journey,<br /><span>more confident.</span></h1><p>Plan your next move with a safety-first navigation workspace built around your journey, your vehicle, and your peace of mind.</p><div className="hero-stats"><div><span className="stat-icon"><Compass size={17} /></span><span><strong>Route planning</strong><small>Provider integration pending</small></span></div><div><span className="stat-icon"><Siren size={17} /></span><span><strong>Emergency ready</strong><small>Configure delivery to activate SOS</small></span></div></div></div>
          <div className="hero-orbit" aria-hidden="true"><div className="orbit orbit-one" /><div className="orbit orbit-two" /><div className="orbit-core"><Navigation size={46} /><span>SR</span></div><span className="orbit-node node-one"><MapPin size={17} /></span><span className="orbit-node node-two"><ShieldCheck size={17} /></span><span className="orbit-node node-three"><Crosshair size={17} /></span></div>
        </section>

        <section className="workspace" id="navigate">
          <div className="section-heading"><div><span className="eyebrow">YOUR JOURNEY</span><h2>Plan a route</h2><p>Choose your travel mode and set your journey details.</p></div><span className="secure-label"><ShieldCheck size={15} /> Privacy-conscious</span></div>
          <div className="planner-grid">
            <form className="panel route-panel" onSubmit={planRoute}>
              <div className="panel-heading"><div className="panel-icon"><Compass size={19} /></div><div><h3>Route planner</h3><p>Origin to destination</p></div></div>
              <label className="field-label" htmlFor="origin">STARTING POINT</label>
              <div className="location-input"><span className="field-pin start"><MapPin size={17} /></span><input id="origin" value={origin} onChange={e => setOrigin(e.target.value)} placeholder="Enter address or coordinates" autoComplete="street-address" /><button type="button" className="field-action" aria-label="Use current location" onClick={useCurrentLocation} disabled={busy}><LocateFixed size={17} /></button></div>
              <div className="field-connector" />
              <label className="field-label" htmlFor="destination">DESTINATION</label>
              <div className="location-input"><span className="field-pin end"><MapPin size={17} /></span><input id="destination" value={destination} onChange={e => setDestination(e.target.value)} placeholder="Where do you want to go?" autoComplete="off" /></div>
              <div className="vehicle-label">TRAVEL MODE</div>
              <div className="vehicle-options">
                {([{id:'car',label:'Car',icon:<CarFront size={18} />},{id:'motorcycle',label:'Two-wheeler',icon:<Bike size={18} />},{id:'truck',label:'Truck',icon:<Truck size={18} />},{id:'walking',label:'Walking',icon:<Footprints size={18} />}] as const).map(item => <button type="button" key={item.id} className={vehicle === item.id ? 'vehicle-option selected' : 'vehicle-option'} onClick={() => setVehicle(item.id)} aria-pressed={vehicle === item.id}>{item.icon}<span>{item.label}</span></button>)}
              </div>
              <button className="primary-button" type="submit"><Navigation size={18} /> Plan journey <ArrowDownUp size={17} /></button>
              {locationMessage && <p className="inline-message" role="status">{locationMessage}</p>}
              <p className="fine-print">Route availability and road restrictions depend on connected provider data. Always follow posted signs and local rules.</p>
            </form>
            <div className="panel map-panel">
              <div className="map-topline"><div><span className="eyebrow">LIVE MAP</span><h3>Journey overview</h3></div><span className="map-chip"><Activity size={14} /> {import.meta.env.VITE_MAPTILER_KEY ? 'Map key provided' : 'Map provider pending'}</span></div>
              <div className={import.meta.env.VITE_MAPTILER_KEY ? 'map-placeholder real-map' : 'map-placeholder'}>{import.meta.env.VITE_MAPTILER_KEY ? <div className="leaflet-map" ref={mapElement} aria-label="Live map from MapTiler" /> : <div className="map-center"><span className="map-center-icon"><Compass size={24} /></span><strong>Map provider not configured</strong><span>Add a valid VITE_MAPTILER_KEY to load real map tiles. No route or live road data is simulated.</span></div>}</div>
              <div className="map-footer"><span><span className="status-dot" /> {database === 'connected' ? 'Database connected' : database === 'not_configured' ? 'Database not configured' : database === 'unavailable' ? 'API unavailable' : 'Checking connection'}</span><span>Mode: {vehicle === 'motorcycle' ? 'Two-wheeler' : vehicle[0].toUpperCase()+vehicle.slice(1)}</span></div>
            </div>
          </div>
        </section>

        <section className="safety-section" id="safety"><div className="section-heading"><div><span className="eyebrow">PREPAREDNESS</span><h2>Your safety center</h2><p>Emergency features must be configured and verified before relying on them.</p></div></div><div className="safety-grid"><article className="panel safety-card"><div className="safety-card-icon"><Siren size={21} /></div><h3>Emergency SOS</h3><p>GPS-backed emergency event workflow. Delivery to contacts is not active until a provider is configured and verified.</p><span className="feature-state"><span className="status-dot" /> Delivery not configured</span></article><article className="panel safety-card"><div className="safety-card-icon"><Users size={21} /></div><h3>Emergency contacts</h3><p>Contact management requires authenticated API endpoints. No contacts are fabricated or prefilled.</p><span className="feature-state"><span className="status-dot" /> Backend endpoints pending</span></article><article className="panel safety-card"><div className="safety-card-icon"><ShieldCheck size={21} /></div><h3>Road awareness</h3><p>Road restrictions and suitability need reliable provider data; this workspace does not claim live safety coverage.</p><span className="feature-state"><span className="status-dot" /> Provider integration pending</span></article></div></section>

        <section className="system-section" id="status"><div className="section-heading"><div><span className="eyebrow">TRANSPARENCY</span><h2>System status</h2><p>Connection status reflects real API responses, not simulated telemetry.</p></div></div><div className="panel status-panel"><div className="status-row"><span className="status-symbol"><Activity size={18} /></span><span><strong>SafeRoute API</strong><small>{apiError || (apiStatus ? 'API responded successfully.' : 'Checking configured API endpoint…')}</small></span><span className={apiStatus ? 'status-value good' : 'status-value'}>{apiStatus ? 'Connected' : 'Unavailable / pending'}</span></div><div className="status-row"><span className="status-symbol"><ShieldCheck size={18} /></span><span><strong>PostgreSQL</strong><small>{database === 'connected' ? 'Database ping succeeded.' : database === 'not_configured' ? 'DATABASE_URL is not set on the API.' : database === 'unavailable' ? 'API could not confirm database connectivity.' : 'Awaiting API response.'}</small></span><span className={database === 'connected' ? 'status-value good' : 'status-value'}>{database}</span></div><div className="status-row"><span className="status-symbol"><MapPin size={18} /></span><span><strong>Routing and map tiles</strong><small>Real routing is not available until the provider key and routing implementation are configured.</small></span><span className="status-value">Not configured</span></div><div className="status-row"><span className="status-symbol"><Bell size={18} /></span><span><strong>SOS delivery provider</strong><small>No SMS, voice, or push provider is currently configured.</small></span><span className="status-value">Not configured</span></div></div></section>
      </main>
      <footer><a className="brand footer-brand" href="#"><span className="brand-mark"><Navigation size={18} /></span><span>SafeRoute<span className="brand-accent">AI</span><small>BUILT FOR MORE CONFIDENT JOURNEYS</small></span></a><p>Navigation assistance only — not a substitute for emergency services or posted road instructions.</p><span>SafeRoute AI • Initial implementation</span></footer>
    </div>
  );
}
