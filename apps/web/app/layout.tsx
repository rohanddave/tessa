import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Tessa RAG",
  description: "Local UI for Tessa RAG service requests"
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <header className="site-header">
          <Link className="brand" href="/">
            Tessa RAG
          </Link>
          <nav aria-label="Primary navigation">
            <Link href="/answer">Answer</Link>
            <Link href="/repo-event">Repo Event</Link>
          </nav>
        </header>
        <main>{children}</main>
      </body>
    </html>
  );
}
