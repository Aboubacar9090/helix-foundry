import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Database } from "lucide-react";
import { Button, ButtonLink, DataGrid, EmptyState, Segmented } from "../../kit";
import { paths } from "../../paths";
import { useWorkspace } from "../../ui";
import { useOntology } from "../ontology/scope";
import { LoadNotice, useLoadSelection } from "../ontology/load-selection";
import {
  graphComputeQuery,
  graphComputeScoresQuery,
  startGraphCompute,
} from "./compute-api";
import { ExplorerTop } from "./top-bar";

const COLUMNS = [
  { key: "nodeKey", label: "Node" },
  { key: "score", label: "Score", type: "number" },
  { key: "rank", label: "Rank", type: "integer" },
];

type Algorithm = "degree" | "wcc" | "pagerank";

const NAMES: Record<Algorithm, string> = {
  degree: "Degree",
  wcc: "Components",
  pagerank: "PageRank",
};

/**
 * /explore/:ontologyId/compute: queue a read-only degree computation over
 * the selected load, poll the run and show its ranked scores. The
 * foundry-graph-compute-worker computes it; the browser never reads the
 * graph for compute.
 */
export function ComputePage() {
  const { id: ws } = useWorkspace();
  const { id: o, ontology } = useOntology();
  const selection = useLoadSelection();
  const [algorithm, setAlgorithm] = useState<Algorithm>("degree");
  const [runId, setRunId] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const load = selection.choice.kind === "load" ? selection.choice.load : null;

  const runQuery = useQuery({
    ...graphComputeQuery(ws, o, runId ?? ""),
    enabled: !!runId,
    refetchInterval: (query) => {
      const status = query.state.data?.run?.status;
      return status === "queued" || status === "running" ? 1500 : false;
    },
  });
  const run = runQuery.data?.run;
  const scoresQuery = useQuery({
    ...graphComputeScoresQuery(ws, o, runId ?? ""),
    enabled: !!runId && run?.status === "succeeded",
  });

  const start = async () => {
    if (!load || starting) return;
    setStarting(true);
    setStartError(null);
    try {
      const created = await startGraphCompute(ws, o, {
        loadId: load.id,
        algorithm,
        name: NAMES[algorithm],
        clientToken: crypto.randomUUID(),
      });
      setRunId(created.run.id);
    } catch (e) {
      setStartError((e as Error).message);
    } finally {
      setStarting(false);
    }
  };

  if (!ontology.tenantId)
    return (
      <div className="ex-view">
        <ExplorerTop view="compute" />
        <div className="ex-frame">
          <EmptyState
            icon={Database}
            title="You can’t read the ontology’s database"
            text="It lives in a project you can’t see. Ask a workspace admin for access to that project."
            action={
              <ButtonLink to={paths.ontologies()}>Go to Ontologies</ButtonLink>
            }
          />
        </div>
      </div>
    );

  if (!load)
    return (
      <div className="ex-view">
        <ExplorerTop view="compute" selection={selection} />
        <div className="ex-frame">
          <LoadNotice selection={selection} />
        </div>
      </div>
    );

  return (
    <div className="ex-view">
      <ExplorerTop view="compute" selection={selection} />
      <div className="ex-frame">
        <section aria-labelledby="compute-degree">
          <h2 id="compute-degree">Degree</h2>
          <p>
            Queues a read-only computation over the selected load. The worker
            reads only that load’s edges and never writes the graph.
          </p>
          <p>
            <Segmented
              label="Algorithm"
              value={algorithm}
              options={[
                { value: "degree", label: "Degree" },
                { value: "pagerank", label: "PageRank" },
                { value: "wcc", label: "Components" },
              ]}
              onChange={(value) => {
                setAlgorithm(value as Algorithm);
                setRunId(null);
              }}
            />
          </p>
          <p>
            <Button onClick={() => void start()} disabled={starting}>
              {starting ? "Queueing…" : `Queue ${NAMES[algorithm]}`}
            </Button>
          </p>
          {startError && <p role="alert">{startError}</p>}
          {run && (
            <p>
              Run <span className="hf-mono">{run.id}</span> · {run.algorithm} ·{" "}
              {run.status} ({run.phase}) · {Number(run.nodeCount ?? 0)} nodes ·{" "}
              {Number(run.edgeCount ?? 0)} edges
            </p>
          )}
          {run?.error && <p role="alert">{run.error}</p>}
          {run &&
            (run.status === "queued" || run.status === "running") && (
              <p>Computing in the background…</p>
            )}
          <DataGrid
            rows={scoresQuery.data?.scores ?? []}
            columns={COLUMNS}
            label="Degree by node"
            loading={
              scoresQuery.isPending &&
              !!runId &&
              run?.status === "succeeded"
            }
            maxHeight={520}
            emptyText={
              run
                ? "No scores yet."
                : "Queue a run to see the nodes with the most edges."
            }
          />
        </section>
      </div>
    </div>
  );
}
