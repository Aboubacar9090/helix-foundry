import { useState } from "react";
import { Database } from "lucide-react";
import { Button, ButtonLink, DataGrid, EmptyState } from "../../kit";
import { paths } from "../../paths";
import { useOntology } from "../ontology/scope";
import { LoadNotice, useLoadSelection } from "../ontology/load-selection";
import {
  buildDegreeDistributionRequest,
  parseDegreeDistribution,
  type DegreeRow,
} from "./explorer/lib/queries";
import { useTenantReads } from "./tenant-reads";
import { ExplorerTop } from "./top-bar";

const COLUMNS = [
  { key: "nodeId", label: "Node" },
  { key: "inDegree", label: "In", type: "integer" },
  { key: "outDegree", label: "Out", type: "integer" },
  { key: "total", label: "Total", type: "integer" },
];

/**
 * /explore/:ontologyId/compute: the first Graph compute view. It reads the
 * selected load's edges through the existing read API, counts in/out degree
 * per node and shows the busiest nodes. The async compute jobs and the other
 * algorithms follow (see PORT-PLAN-graph-go-nextjs.md).
 */
export function ComputePage() {
  const { ontology } = useOntology();
  const selection = useLoadSelection();
  const reads = useTenantReads();
  const [rows, setRows] = useState<DegreeRow[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const scope = selection.scope;

  const run = async () => {
    if (!scope || busy) return;
    setBusy(true);
    setError(null);
    try {
      const result = await reads.read(buildDegreeDistributionRequest(scope));
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setRows(parseDegreeDistribution(result.body));
    } finally {
      setBusy(false);
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

  if (!scope)
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
            Counts each node’s in and out edges in the selected load. It reads
            only that load’s edges and never writes.
          </p>
          <p>
            <Button onClick={() => void run()} disabled={busy}>
              {busy ? "Computing…" : "Run degree"}
            </Button>{" "}
            {rows && <span>{rows.length} nodes</span>}
          </p>
          {error && <p role="alert">{error}</p>}
          <DataGrid
            rows={rows ?? []}
            columns={COLUMNS}
            label="Degree by node"
            loading={busy}
            maxHeight={520}
            emptyText="Run degree to see the nodes with the most edges."
          />
        </section>
      </div>
    </div>
  );
}
