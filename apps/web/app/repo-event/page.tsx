"use client";

import { FormEvent, useMemo, useState } from "react";

type ProxyResult = {
  ok: boolean;
  status: number;
  data: unknown;
};

const initialForm = {
  repo_url: "",
  provider: "github",
  branch: "main",
  commit_sha: "",
  event_type: "repo.created",
  requested_by: "local"
};

export default function RepoEventPage() {
  const [form, setForm] = useState(initialForm);
  const [result, setResult] = useState<ProxyResult | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const payload = useMemo(
    () => ({
      repo_url: form.repo_url,
      provider: form.provider,
      branch: form.branch,
      commit_sha: form.commit_sha,
      event_type: form.event_type,
      requested_by: form.requested_by
    }),
    [form]
  );

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsSubmitting(true);
    setResult(null);

    try {
      const response = await fetch("/api/repo-event", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify(payload)
      });
      const data = (await response.json()) as ProxyResult;
      setResult(data);
    } catch (error) {
      setResult({
        ok: false,
        status: 0,
        data: {
          error: error instanceof Error ? error.message : "Request failed"
        }
      });
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section className="workspace">
      <div className="page-heading">
        <p className="eyebrow">POST /repo-event</p>
        <h1>Publish a repository event</h1>
      </div>

      <div className="request-layout">
        <form className="request-form" onSubmit={onSubmit}>
          <label>
            Repository URL
            <input
              required
              value={form.repo_url}
              onChange={(event) =>
                setForm({ ...form, repo_url: event.target.value })
              }
              placeholder="https://github.com/example/repo.git"
              type="url"
            />
          </label>

          <div className="field-row">
            <label>
              Provider
              <select
                value={form.provider}
                onChange={(event) =>
                  setForm({ ...form, provider: event.target.value })
                }
              >
                <option value="github">github</option>
                <option value="gitlab">gitlab</option>
                <option value="bitbucket">bitbucket</option>
              </select>
            </label>
            <label>
              Event Type
              <select
                value={form.event_type}
                onChange={(event) =>
                  setForm({ ...form, event_type: event.target.value })
                }
              >
                <option value="repo.created">repo.created</option>
                <option value="repo.updated">repo.updated</option>
                <option value="repo.deleted">repo.deleted</option>
              </select>
            </label>
          </div>

          <div className="field-row">
            <label>
              Branch
              <input
                value={form.branch}
                onChange={(event) =>
                  setForm({ ...form, branch: event.target.value })
                }
                placeholder="main"
              />
            </label>
            <label>
              Commit SHA
              <input
                value={form.commit_sha}
                onChange={(event) =>
                  setForm({ ...form, commit_sha: event.target.value })
                }
                placeholder="optional"
              />
            </label>
          </div>

          <label>
            Requested By
            <input
              required
              value={form.requested_by}
              onChange={(event) =>
                setForm({ ...form, requested_by: event.target.value })
              }
              placeholder="local"
            />
          </label>

          <button disabled={isSubmitting} type="submit">
            {isSubmitting ? "Sending..." : "Send repo event"}
          </button>
        </form>

        <ResponsePanel payload={payload} result={result} />
      </div>
    </section>
  );
}

function ResponsePanel({
  payload,
  result
}: {
  payload: unknown;
  result: ProxyResult | null;
}) {
  return (
    <aside className="response-panel">
      <div>
        <h2>Request JSON</h2>
        <pre>{JSON.stringify(payload, null, 2)}</pre>
      </div>
      <div>
        <h2>Response</h2>
        <pre>
          {result
            ? JSON.stringify(result, null, 2)
            : "Submit the form to see the service response."}
        </pre>
      </div>
    </aside>
  );
}
