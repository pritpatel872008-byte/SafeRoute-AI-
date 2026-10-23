CREATE TABLE IF NOT EXISTS safety_reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  submitted_by UUID REFERENCES app_users(id) ON DELETE SET NULL,
  title TEXT NOT NULL CHECK (char_length(title) BETWEEN 5 AND 180),
  description TEXT NOT NULL CHECK (char_length(description) BETWEEN 10 AND 3000),
  category TEXT NOT NULL CHECK (category IN ('accident','roadblock','hazard','crime','weather','other')),
  latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
  longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
  source_url TEXT,
  source_name TEXT NOT NULL DEFAULT 'Community report',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
  external_id TEXT UNIQUE,
  occurred_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reviewed_at TIMESTAMPTZ,
  reviewed_by UUID REFERENCES app_users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS safety_reports_status_created_idx ON safety_reports(status, created_at DESC);
CREATE INDEX IF NOT EXISTS safety_reports_geo_idx ON safety_reports(latitude, longitude);
