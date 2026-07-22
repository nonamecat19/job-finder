// API_BASE_URL is the job-finder API origin the extension talks to. It is a
// build-time constant (not user-configurable at runtime) so it stays in
// lockstep with the manifest's host_permissions — an extension can only
// fetch an origin it has host permission for, so changing this without
// updating vite.config.ts's manifest would silently break auth.
//
// Dev default matches apps/api's default PORT (config.go: PORT=3000). A
// packaged production build should override this at build time (e.g. via a
// separate vite.config.prod.ts or an env-driven define) rather than ship
// with localhost baked in.
export const API_BASE_URL = 'http://localhost:3000/api'
