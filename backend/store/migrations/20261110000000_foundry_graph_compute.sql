-- +goose Up
-- Graph compute runs are read-only jobs over one load: they count or score
-- the load's nodes without writing the graph. They have their own queue so a
-- long PageRank never holds the load writer's lease. Results live in S3
-- (result_uri) and in the score/finding tables below for the UI.
CREATE TABLE foundry_graph_compute_runs (
  id                   CHAR(26)      NOT NULL PRIMARY KEY,
  workspace_id         CHAR(26)      NOT NULL,
  ontology_id          CHAR(26)      NOT NULL,
  load_id              CHAR(26)      NOT NULL,
  algorithm            VARCHAR(32)   NOT NULL,
  name                 VARCHAR(128)  NOT NULL,
  params               JSON          NULL,
  status               VARCHAR(16)   NOT NULL,
  phase                VARCHAR(16)   NOT NULL,
  client_token         VARCHAR(64)   NULL,
  lease_owner          VARCHAR(64)   NULL,
  lease_until          DATETIME(6)   NULL,
  attempts             INT           NOT NULL DEFAULT 0,
  next_attempt_at      DATETIME(6)   NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  error                TEXT          NULL,
  node_count           BIGINT        NOT NULL DEFAULT 0,
  edge_count           BIGINT        NOT NULL DEFAULT 0,
  top_score            DOUBLE        NULL,
  result_uri           VARCHAR(1024) NULL,
  requested_by_user_id CHAR(26)      NULL,
  created_at           DATETIME(6)   NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  started_at           DATETIME(6)   NULL,
  finished_at          DATETIME(6)   NULL,
  UNIQUE KEY foundry_graph_compute_runs_client_token_uq (ontology_id, client_token),
  INDEX foundry_graph_compute_runs_claim_idx (status, next_attempt_at),
  INDEX foundry_graph_compute_runs_history_idx (ontology_id, created_at, id),
  INDEX foundry_graph_compute_runs_load_idx (load_id, created_at, id),
  CONSTRAINT foundry_graph_compute_runs_status_chk CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
  CONSTRAINT foundry_graph_compute_runs_phase_chk CHECK (phase IN ('queued', 'computing', 'writing', 'finishing'))
);

-- One row per node score, ranked for the results table. The full result stays
-- in S3; this table serves the first pages and the top-k.
CREATE TABLE foundry_graph_compute_scores (
  run_id   CHAR(26)     NOT NULL,
  node_key VARCHAR(512) NOT NULL,
  score    DOUBLE       NOT NULL,
  rank_no  INT          NOT NULL,
  extra    JSON         NULL,
  PRIMARY KEY (run_id, node_key),
  INDEX foundry_graph_compute_scores_rank_idx (run_id, rank_no)
);

-- One row per fraud finding, for the recipe views.
CREATE TABLE foundry_graph_compute_findings (
  run_id      CHAR(26)     NOT NULL,
  finding_id  VARCHAR(128) NOT NULL,
  recipe      VARCHAR(32)  NOT NULL,
  severity    VARCHAR(16)  NOT NULL,
  score       DOUBLE       NOT NULL,
  node_keys   JSON         NULL,
  amount      BIGINT       NULL,
  explanation TEXT         NULL,
  PRIMARY KEY (run_id, finding_id),
  INDEX foundry_graph_compute_findings_severity_idx (run_id, severity, score)
);

-- +goose Down
DROP TABLE foundry_graph_compute_findings;
DROP TABLE foundry_graph_compute_scores;
DROP TABLE foundry_graph_compute_runs;
