import { forwardJson } from "@/lib/proxy";

const queryServiceUrl =
  process.env.QUERY_SERVICE_URL?.replace(/\/$/, "") ?? "http://localhost:8082";

export async function POST(request: Request) {
  return forwardJson(request, queryServiceUrl, "/answer");
}
