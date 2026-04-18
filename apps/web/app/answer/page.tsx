"use client";

import { useMemo, useState } from "react";

type ProxyResult = {
  ok: boolean;
  status: number;
  data: unknown;
};

type MarkdownBlock =
  | { type: "heading"; depth: number; text: string }
  | { type: "paragraph"; text: string }
  | { type: "blockquote"; text: string }
  | { type: "list"; ordered: boolean; items: string[] }
  | { type: "code"; language: string; text: string };

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
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
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

  async function submitRequest() {
    const nextErrors = validateForm(form);
    setFormErrors(nextErrors);

    if (Object.keys(nextErrors).length > 0) {
      return;
    }

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
        <div
          aria-label="Answer request form"
          className="request-form"
          role="form"
        >
          <label>
            Query
            <textarea
              aria-invalid={Boolean(formErrors.query)}
              rows={5}
              value={form.query}
              onChange={(event) =>
                setForm({ ...form, query: event.target.value })
              }
              placeholder="Explain the invoice creation flow"
            />
            {formErrors.query ? (
              <span className="field-error">{formErrors.query}</span>
            ) : null}
          </label>

          <label>
            Repository URL
            <input
              aria-invalid={Boolean(formErrors.repo_url)}
              value={form.repo_url}
              onChange={(event) =>
                setForm({ ...form, repo_url: event.target.value })
              }
              placeholder="https://github.com/example/repo.git"
              type="url"
            />
            {formErrors.repo_url ? (
              <span className="field-error">{formErrors.repo_url}</span>
            ) : null}
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
                aria-invalid={Boolean(formErrors.top_k)}
                min={1}
                max={50}
                value={form.top_k}
                onChange={(event) =>
                  setForm({ ...form, top_k: Number(event.target.value) })
                }
                type="number"
              />
              {formErrors.top_k ? (
                <span className="field-error">{formErrors.top_k}</span>
              ) : null}
            </label>
            <label>
              Token Budget
              <input
                aria-invalid={Boolean(formErrors.context_token_budget)}
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
              {formErrors.context_token_budget ? (
                <span className="field-error">
                  {formErrors.context_token_budget}
                </span>
              ) : null}
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

          <button
            disabled={isSubmitting}
            onClick={() => void submitRequest()}
            type="button"
          >
            {isSubmitting ? "Sending..." : "Send answer request"}
          </button>
        </div>

        <ResponsePanel isLoading={isSubmitting} result={result} />
      </div>
    </section>
  );
}

function ResponsePanel({
  isLoading,
  result
}: {
  isLoading: boolean;
  result: ProxyResult | null;
}) {
  const [activeTab, setActiveTab] = useState<"answer" | "stats">("answer");
  const answer = getAnswerText(result);
  const stats = getStats(result);

  return (
    <aside className="response-panel">
      <div className="response-tabs" role="tablist" aria-label="Answer response">
        <button
          aria-selected={activeTab === "answer"}
          className="tab-button"
          onClick={() => setActiveTab("answer")}
          role="tab"
          type="button"
        >
          Answer
        </button>
        <button
          aria-selected={activeTab === "stats"}
          className="tab-button"
          onClick={() => setActiveTab("stats")}
          role="tab"
          type="button"
        >
          Stats
        </button>
      </div>

      {activeTab === "answer" ? (
        <div className="tab-panel" role="tabpanel">
          {isLoading ? (
            <LoadingState />
          ) : answer ? (
            <MarkdownViewer content={answer} />
          ) : (
            <p className="empty-response">
              Submit the form to see the markdown answer.
            </p>
          )}
        </div>
      ) : (
        <div className="tab-panel" role="tabpanel">
          {isLoading ? (
            <LoadingState />
          ) : (
            <pre>
              {stats
                ? JSON.stringify(stats, null, 2)
                : "Submit the form to see request status, citations, context, limitations, and token usage."}
            </pre>
          )}
        </div>
      )}
    </aside>
  );
}

function LoadingState() {
  return (
    <div className="loading-state" role="status" aria-live="polite">
      <span className="loader" aria-hidden="true" />
      <span>Waiting for Tessa...</span>
    </div>
  );
}

function validateForm(form: typeof initialForm) {
  const errors: Record<string, string> = {};
  const repoUrl = form.repo_url.trim();

  if (!form.query.trim()) {
    errors.query = "Enter a question.";
  }

  if (!repoUrl) {
    errors.repo_url = "Repository URL is required.";
  } else {
    try {
      const url = new URL(repoUrl);
      if (!["http:", "https:"].includes(url.protocol)) {
        errors.repo_url = "Use an http or https repository URL.";
      }
    } catch {
      errors.repo_url = "Enter a valid repository URL.";
    }
  }

  if (Number(form.top_k) < 1 || Number(form.top_k) > 50) {
    errors.top_k = "Top K must be between 1 and 50.";
  }

  if (
    Number(form.context_token_budget) < 1000 ||
    Number(form.context_token_budget) > 50000
  ) {
    errors.context_token_budget =
      "Token budget must be between 1,000 and 50,000.";
  }

  return errors;
}

function getAnswerText(result: ProxyResult | null) {
  if (!result) {
    return "";
  }

  if (isRecord(result.data) && typeof result.data.answer === "string") {
    return result.data.answer;
  }

  if (!result.ok) {
    return "The request did not return an answer.";
  }

  return "";
}

function getStats(result: ProxyResult | null) {
  if (!result) {
    return null;
  }

  if (!isRecord(result.data)) {
    return result;
  }

  const { answer: _answer, ...remainingData } = result.data;
  return {
    ok: result.ok,
    status: result.status,
    data: remainingData
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function MarkdownViewer({ content }: { content: string }) {
  const blocks = parseMarkdown(content);

  return (
    <div className="markdown-viewer">
      {blocks.map((block, index) => {
        if (block.type === "heading") {
          const HeadingTag = `h${Math.min(block.depth + 2, 6)}` as keyof JSX.IntrinsicElements;
          return <HeadingTag key={index}>{renderInline(block.text)}</HeadingTag>;
        }

        if (block.type === "blockquote") {
          return <blockquote key={index}>{renderInline(block.text)}</blockquote>;
        }

        if (block.type === "list") {
          const ListTag = block.ordered ? "ol" : "ul";
          return (
            <ListTag key={index}>
              {block.items.map((item, itemIndex) => (
                <li key={itemIndex}>{renderInline(item)}</li>
              ))}
            </ListTag>
          );
        }

        if (block.type === "code") {
          return (
            <pre className="markdown-code" key={index}>
              <code>{block.text}</code>
            </pre>
          );
        }

        return <p key={index}>{renderInline(block.text)}</p>;
      })}
    </div>
  );
}

function parseMarkdown(content: string): MarkdownBlock[] {
  const blocks: MarkdownBlock[] = [];
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];

    if (!line.trim()) {
      index += 1;
      continue;
    }

    const fence = line.match(/^```(\w+)?\s*$/);
    if (fence) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !lines[index].startsWith("```")) {
        codeLines.push(lines[index]);
        index += 1;
      }
      blocks.push({
        type: "code",
        language: fence[1] ?? "",
        text: codeLines.join("\n")
      });
      index += 1;
      continue;
    }

    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      blocks.push({
        type: "heading",
        depth: heading[1].length,
        text: heading[2]
      });
      index += 1;
      continue;
    }

    if (line.startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length && lines[index].startsWith(">")) {
        quoteLines.push(lines[index].replace(/^>\s?/, ""));
        index += 1;
      }
      blocks.push({ type: "blockquote", text: quoteLines.join(" ") });
      continue;
    }

    const unordered = line.match(/^\s*[-*]\s+(.+)$/);
    const ordered = line.match(/^\s*\d+\.\s+(.+)$/);
    if (unordered || ordered) {
      const orderedList = Boolean(ordered);
      const items: string[] = [];
      while (index < lines.length) {
        const item = orderedList
          ? lines[index].match(/^\s*\d+\.\s+(.+)$/)
          : lines[index].match(/^\s*[-*]\s+(.+)$/);
        if (!item) {
          break;
        }
        items.push(item[1]);
        index += 1;
      }
      blocks.push({ type: "list", ordered: orderedList, items });
      continue;
    }

    const paragraphLines: string[] = [line.trim()];
    index += 1;
    while (index < lines.length && lines[index].trim()) {
      if (
        /^```/.test(lines[index]) ||
        /^(#{1,4})\s+/.test(lines[index]) ||
        lines[index].startsWith(">") ||
        /^\s*([-*]|\d+\.)\s+/.test(lines[index])
      ) {
        break;
      }
      paragraphLines.push(lines[index].trim());
      index += 1;
    }
    blocks.push({ type: "paragraph", text: paragraphLines.join(" ") });
  }

  return blocks;
}

function renderInline(text: string) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g);

  return parts.map((part, index) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return <code key={index}>{part.slice(1, -1)}</code>;
    }

    if (part.startsWith("**") && part.endsWith("**")) {
      return <strong key={index}>{part.slice(2, -2)}</strong>;
    }

    return part;
  });
}
