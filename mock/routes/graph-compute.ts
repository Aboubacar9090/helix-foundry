import type {
  GraphComputeRun,
  GraphComputeScore,
  ListGraphComputeScoresResponse,
  ListGraphComputesResponse,
} from "../../src/shared/wfe";
import { fail, id, now, ok, type MockRoute } from "../http";
import { isReply, membership, onReset } from "../state";

// Read-only graph computations over a load, as the Foundry API serves them.
// A run finishes immediately here with a small degree table, so the Compute
// view can show its scores without a worker.

const base = "/v1/workspaces/{workspaceId}/foundry/ontologies/{ontologyId}/graph/compute";

const runs = new Map<string, GraphComputeRun>();
const scores = new Map<string, GraphComputeScore[]>();
onReset(() => {
  runs.clear();
  scores.clear();
});

const demo: Record<string, GraphComputeScore[]> = {
  degree: [
    { nodeKey: "a", score: 2, rank: 1 },
    { nodeKey: "b", score: 1, rank: 2 },
    { nodeKey: "c", score: 1, rank: 3 },
  ],
  pagerank: [
    { nodeKey: "a", score: 0.4, rank: 1 },
    { nodeKey: "b", score: 0.35, rank: 2 },
    { nodeKey: "c", score: 0.25, rank: 3 },
  ],
  wcc: [
    { nodeKey: "a", score: 3, rank: 1 },
    { nodeKey: "b", score: 3, rank: 2 },
    { nodeKey: "c", score: 3, rank: 3 },
  ],
};

export const graphComputeRoutes: MockRoute[] = [
  {
    method: "POST",
    path: base,
    handle: ({ params, body, principal }) => {
      const m = membership(params.workspaceId!, principal.user.id);
      if (isReply(m)) return m;
      if (!body?.loadId) return fail("INVALID_ARGUMENT", "loadId is required");
      if (!["degree", "wcc", "pagerank"].includes(body.algorithm))
        return fail(
          "INVALID_ARGUMENT",
          "algorithm must be degree, wcc or pagerank for now",
        );
      const table = demo[body.algorithm] ?? demo.degree!;
      const run: GraphComputeRun = {
        id: id("run"),
        workspaceId: params.workspaceId!,
        ontologyId: params.ontologyId!,
        loadId: body.loadId,
        algorithm: body.algorithm,
        name: body.name ?? body.algorithm,
        status: "succeeded",
        phase: "done",
        nodeCount: String(table.length),
        edgeCount: "3",
        topScore: table[0]?.score ?? 0,
        createdAt: now(),
        startedAt: now(),
        finishedAt: now(),
      };
      runs.set(run.id, run);
      scores.set(run.id, table);
      return ok({ run });
    },
  },
  {
    method: "GET",
    path: base,
    handle: ({ params, principal }) => {
      const m = membership(params.workspaceId!, principal.user.id);
      if (isReply(m)) return m;
      const list = [...runs.values()].filter(
        (run) => run.ontologyId === params.ontologyId,
      );
      return ok({ runs: list } satisfies ListGraphComputesResponse);
    },
  },
  {
    method: "GET",
    path: `${base}/{runId}/scores`,
    handle: ({ params, principal }) => {
      const m = membership(params.workspaceId!, principal.user.id);
      if (isReply(m)) return m;
      return ok({
        scores: scores.get(params.runId!) ?? [],
      } satisfies ListGraphComputeScoresResponse);
    },
  },
  {
    method: "GET",
    path: `${base}/{runId}`,
    handle: ({ params, principal }) => {
      const m = membership(params.workspaceId!, principal.user.id);
      if (isReply(m)) return m;
      const run = runs.get(params.runId!);
      if (!run) return fail("NOT_FOUND", "graph compute run not found");
      return ok({ run });
    },
  },
];
