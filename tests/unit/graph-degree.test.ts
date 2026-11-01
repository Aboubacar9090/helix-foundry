import { describe, expect, it } from "vitest";
import {
  buildDegreeDistributionRequest,
  parseDegreeDistribution,
} from "@/web/areas/explore/explorer/lib/queries";

describe("graph degree", () => {
  it("reads out and in degrees over the load's edges", () => {
    const request = buildDegreeDistributionRequest({
      kind: "load",
      id: "load-1",
    });
    expect(request.request_type).toBe("read");
    expect(request.query_name).toBe("graph_degree");
    const entries = (request.query as any).read.entries;
    expect(entries.map((e: any) => e.query.name)).toEqual([
      "out_degrees",
      "in_degrees",
    ]);
    expect(JSON.stringify(entries[0])).toContain("foundry_load");
    expect(JSON.stringify(entries[0])).toContain("load-1");
    expect(JSON.stringify(entries[0])).toContain("$from");
    expect(JSON.stringify(entries[1])).toContain("$to");
  });

  it("merges both sides and sorts by total, then node id", () => {
    const rows = parseDegreeDistribution({
      out_degrees: [
        { $from: 1, count: 2 },
        { $from: 2, count: 1 },
      ],
      in_degrees: [
        { $to: 1, count: 1 },
        { $to: 3, count: 1 },
      ],
    });
    expect(rows).toEqual([
      { nodeId: "1", inDegree: 1, outDegree: 2, total: 3 },
      { nodeId: "2", inDegree: 0, outDegree: 1, total: 1 },
      { nodeId: "3", inDegree: 1, outDegree: 0, total: 1 },
    ]);
  });

  it("ignores malformed group-count rows", () => {
    expect(
      parseDegreeDistribution({
        out_degrees: [{ $from: 1, count: "2" }, { $from: 2 }],
        in_degrees: null,
      }),
    ).toEqual([]);
  });
});
