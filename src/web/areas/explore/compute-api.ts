import type {
  GraphComputeRunResponse,
  ListGraphComputeScoresResponse,
  ListGraphComputesResponse,
  StartGraphComputeRequest,
} from "../../../shared/wfe";
import { api } from "../../api";

// The graph compute queue: a read-only run over one load, computed by the
// foundry-graph-compute-worker. The browser only starts a run and polls it;
// it never reads the graph itself for compute.

const base = (workspaceId: string, ontologyId: string) =>
  `/v1/workspaces/${workspaceId}/foundry/ontologies/${ontologyId}/graph/compute`;

export function startGraphCompute(
  workspaceId: string,
  ontologyId: string,
  body: StartGraphComputeRequest,
) {
  return api<GraphComputeRunResponse>(
    base(workspaceId, ontologyId),
    body,
    "POST",
  );
}

/** One run; the page polls it while it is queued or running. */
export const graphComputeQuery = (
  workspaceId: string,
  ontologyId: string,
  runId: string,
) => ({
  queryKey: [workspaceId, "ontology", ontologyId, "compute", runId] as const,
  queryFn: () =>
    api<GraphComputeRunResponse>(`${base(workspaceId, ontologyId)}/${runId}`),
});

/** The ontology's runs, newest first. */
export const graphComputesQuery = (
  workspaceId: string,
  ontologyId: string,
) => ({
  queryKey: [workspaceId, "ontology", ontologyId, "compute"] as const,
  queryFn: () =>
    api<ListGraphComputesResponse>(base(workspaceId, ontologyId)),
});

/** A run's ranked scores, a page at a time. */
export const graphComputeScoresQuery = (
  workspaceId: string,
  ontologyId: string,
  runId: string,
  limit = 200,
  offset = 0,
) => ({
  queryKey: [
    workspaceId,
    "ontology",
    ontologyId,
    "compute",
    runId,
    "scores",
    limit,
    offset,
  ] as const,
  queryFn: () =>
    api<ListGraphComputeScoresResponse>(
      `${base(workspaceId, ontologyId)}/${runId}/scores?limit=${limit}&offset=${offset}`,
    ),
});
