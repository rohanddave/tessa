import Link from "next/link";

export default function Home() {
  return (
    <section className="intro">
      <div>
        <p className="eyebrow">Local request console</p>
        <h1>Ask indexed code, or send a repository event.</h1>
        <p className="lead">
          Choose the workflow you need. Each page sends JSON through a Next.js
          proxy route to the matching Tessa service.
        </p>
      </div>
      <div className="choice-grid" aria-label="Available request pages">
        <Link className="choice" href="/answer">
          <span>Ask a question</span>
          <small>POST /answer</small>
        </Link>
        <Link className="choice" href="/repo-event">
          <span>Send repo event</span>
          <small>POST /repo-event</small>
        </Link>
      </div>
    </section>
  );
}
