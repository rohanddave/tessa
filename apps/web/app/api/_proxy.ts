import { NextResponse } from "next/server";

export type ProxyResult = {
  ok: boolean;
  status: number;
  data: unknown;
};

export async function forwardJson(
  request: Request,
  baseUrl: string,
  endpoint: string
) {
  try {
    const body = await request.json();
    const response = await fetch(`${baseUrl}${endpoint}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(body),
      cache: "no-store"
    });

    const contentType = response.headers.get("content-type") ?? "";
    const data = contentType.includes("application/json")
      ? await response.json()
      : await response.text();

    return NextResponse.json<ProxyResult>(
      {
        ok: response.ok,
        status: response.status,
        data
      },
      { status: response.ok ? 200 : response.status }
    );
  } catch (error) {
    return NextResponse.json<ProxyResult>(
      {
        ok: false,
        status: 500,
        data: {
          error: error instanceof Error ? error.message : "Request failed"
        }
      },
      { status: 500 }
    );
  }
}
