import { forwardJson } from "../_proxy";

const repoSyncServiceUrl =
  process.env.REPO_SYNC_SERVICE_URL?.replace(/\/$/, "") ??
  "http://localhost:8081";

export async function POST(request: Request) {
  return forwardJson(request, repoSyncServiceUrl, "/repo-event");
}
