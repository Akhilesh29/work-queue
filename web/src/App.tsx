import { useCallback, useEffect, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined)?.replace(/\/$/, "") ?? "";
const WORKER_BASE = (import.meta.env.VITE_WORKER_URL as string | undefined)?.replace(/\/$/, "") ?? "";

type Job = {
  id: string;
  type: string;
  status: string;
  retries: number;
  attempts: number;
  payload: unknown;
  error_message?: string | null;
  created_at: string;
  updated_at: string;
};

type Metrics = {
  total_jobs_in_queue: number;
  jobs_done: number;
  jobs_failed: number;
  worker_concurrency: number;
  queue_name: string;
};

function apiPath(path: string): string {
  if (API_BASE) return `${API_BASE}${path}`;
  return path;
}

function workerPath(path: string): string {
  if (WORKER_BASE) return `${WORKER_BASE}${path}`;
  return path;
}

export default function App() {
  const [type, setType] = useState("send_email");
  const [retries, setRetries] = useState(3);
  const [payloadText, setPayloadText] = useState(
    '{\n  "to": "you@example.com",\n  "subject": "Hello from WorkQueue"\n}'
  );
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [metrics, setMetrics] = useState<Metrics | null>(null);

  const loadJobs = useCallback(async () => {
    const res = await fetch(apiPath("/api/jobs"));
    if (!res.ok) return;
    const data = (await res.json()) as Job[];
    setJobs(Array.isArray(data) ? data : []);
  }, []);

  const loadMetrics = useCallback(async () => {
    const res = await fetch(workerPath("/metrics"));
    if (!res.ok) return;
    setMetrics((await res.json()) as Metrics);
  }, []);

  useEffect(() => {
    void loadJobs();
    void loadMetrics();
    const t = setInterval(() => {
      void loadJobs();
      void loadMetrics();
    }, 4000);
    return () => clearInterval(t);
  }, [loadJobs, loadMetrics]);

  async function onEnqueue(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setMsg(null);
    let payload: Record<string, unknown>;
    try {
      payload = JSON.parse(payloadText) as Record<string, unknown>;
    } catch {
      setErr("Payload must be valid JSON.");
      return;
    }
    setLoading(true);
    try {
      const res = await fetch(apiPath("/enqueue"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type, retries, payload }),
      });
      const text = await res.text();
      if (!res.ok) {
        setErr(text || res.statusText);
        return;
      }
      try {
        const parsed = JSON.parse(text) as { job_id?: string; status?: string; type?: string };
        const id = parsed.job_id ? ` (job_id: ${parsed.job_id})` : "";
        setMsg(`Queued${id}`);
      } catch {
        setMsg("Queued");
      }
      await loadJobs();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="container">
      <header>
        <h1>WorkQueue</h1>
        <p className="thin">Queue jobs and monitor execution in real-time.</p>
      </header>

      <div className="grid">
        <div className="card">
          <h2>New job</h2>
          <form onSubmit={onEnqueue}>
            <div className="row">
              <label htmlFor="type">Type</label>
              <select id="type" value={type} onChange={(e) => setType(e.target.value)}>
                <option value="send_email">send_email</option>
                <option value="resize_image">resize_image</option>
                <option value="generate_pdf">generate_pdf</option>
              </select>
            </div>
            <div className="row">
              <label htmlFor="retries">Retries</label>
              <input
                id="retries"
                type="number"
                min={0}
                value={retries}
                onChange={(e) => setRetries(Number(e.target.value))}
              />
            </div>
            <div className="row">
              <label htmlFor="payload">Payload JSON</label>
              <textarea id="payload" value={payloadText} onChange={(e) => setPayloadText(e.target.value)} />
            </div>
            <button type="submit" className="primary" disabled={loading}>
              {loading ? "Enqueueing…" : "Enqueue"}
            </button>
          </form>
          {err ? <div className="err">{err}</div> : null}
          {msg ? <div className="ok">{msg}</div> : null}
        </div>

        <div className="card">
          <h2>Worker metrics</h2>
          {metrics ? (
            <div className="metrics">
              <div>
                <div className="thin">Queue depth</div>
                <strong>{metrics.total_jobs_in_queue}</strong>
              </div>
              <div>
                <div className="thin">Done</div>
                <strong>{metrics.jobs_done}</strong>
              </div>
              <div>
                <div className="thin">Failed</div>
                <strong>{metrics.jobs_failed}</strong>
              </div>
              <div>
                <div className="thin">Concurrency</div>
                <strong>{metrics.worker_concurrency}</strong>
              </div>
            </div>
          ) : (
            <p className="muted">No metrics yet. Set VITE_WORKER_URL to your worker HTTPS URL on Vercel.</p>
          )}
        </div>
      </div>

      <div className="card" style={{ marginTop: "1.25rem" }}>
        <h2>Recent jobs (Postgres)</h2>
        {jobs.length === 0 ? (
          <p className="muted">No jobs yet.</p>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Type</th>
                  <th>Attempts</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => (
                  <tr key={j.id}>
                    <td>
                      <span className={`badge ${j.status}`}>{j.status}</span>
                    </td>
                    <td>{j.type}</td>
                    <td>
                      {j.attempts}/{j.retries}
                    </td>
                    <td className="thin">{new Date(j.updated_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
