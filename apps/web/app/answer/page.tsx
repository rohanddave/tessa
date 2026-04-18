"use client";

import { FormEvent, useMemo, useState } from "react";

type ProxyResult = {
  ok: boolean;
  status: number;
  data: unknown;
};

const initialForm = {
  query: "",
  repo_url: "",
  branch: "main",
  snapshot_id: "",
  top_k: 8,
  context_token_budget: 12000,
  mode: "baseline",
  stream: false
};

export default function AnswerPage() {
  const [form, setForm] = useState(initialForm);
  const [result, setResult] = useState<ProxyResult | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const payload = useMemo(
    () => ({
      query: form.query,
      repo_url: form.repo_url || undefined,
      branch: form.branch || "main",
      snapshot_id: form.snapshot_id || undefined,
      top_k: Number(form.top_k),
      context_token_budget: Number(form.context_token_budget),
      mode: form.mode,
      stream: form.stream
    }),
    [form]
  );

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsSubmitting(true);
    setResult(null);

    try {
      const response = await fetch("/api/answer", {
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
        <p className="eyebrow">POST /answer</p>
        <h1>Ask Tessa a question</h1>
      </div>

      <div className="request-layout">
        <form className="request-form" onSubmit={onSubmit}>
          <label>
            Query
            <textarea
              required
              rows={5}
              value={form.query}
              onChange={(event) =>
                setForm({ ...form, query: event.target.value })
              }
              placeholder="Explain the invoice creation flow"
            />
          </label>

          <label>
            Repository URL
            <input
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
              Snapshot ID
              <input
                value={form.snapshot_id}
                onChange={(event) =>
                  setForm({ ...form, snapshot_id: event.target.value })
                }
                placeholder="optional"
              />
            </label>
          </div>

          <div className="field-row">
            <label>
              Top K
              <input
                min={1}
                max={50}
                value={form.top_k}
                onChange={(event) =>
                  setForm({ ...form, top_k: Number(event.target.value) })
                }
                type="number"
              />
            </label>
            <label>
              Token Budget
              <input
                min={1000}
                max={50000}
                step={500}
                value={form.context_token_budget}
                onChange={(event) =>
                  setForm({
                    ...form,
                    context_token_budget: Number(event.target.value)
                  })
                }
                type="number"
              />
            </label>
          </div>

          <div className="field-row">
            <label>
              Mode
              <select
                value={form.mode}
                onChange={(event) =>
                  setForm({ ...form, mode: event.target.value })
                }
              >
                <option value="baseline">baseline</option>
                <option value="reasoning">reasoning</option>
                <option value="agentic">agentic</option>
              </select>
            </label>
            <label className="checkbox-label">
              <input
                checked={form.stream}
                onChange={(event) =>
                  setForm({ ...form, stream: event.target.checked })
                }
                type="checkbox"
              />
              Stream
            </label>
          </div>

          <button disabled={isSubmitting} type="submit">
            {isSubmitting ? "Sending..." : "Send answer request"}
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
