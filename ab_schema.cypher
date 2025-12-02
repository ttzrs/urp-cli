// ═══════════════════════════════════════════════════════════════════════════════
// A/B TESTING SCHEMA FOR MEMGRAPH
// ═══════════════════════════════════════════════════════════════════════════════
// Run this file to initialize the schema:
//   cat ab_schema.cypher | docker exec -i urp-memgraph mgconsole

// ─────────────────────────────────────────────────────────────────────────────────
// INDEXES for performance
// ─────────────────────────────────────────────────────────────────────────────────

CREATE INDEX ON :ABSession(session_id);
CREATE INDEX ON :ABContainer(container_id);
CREATE INDEX ON :ABContainer(mode);
CREATE INDEX ON :ABContainer(session_id);
CREATE INDEX ON :ABMetric(metric_type);
CREATE INDEX ON :ABResult(mode);

// ─────────────────────────────────────────────────────────────────────────────────
// CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────────────

// Unique session IDs
CREATE CONSTRAINT ON (s:ABSession) ASSERT s.session_id IS UNIQUE;

// Unique container IDs
CREATE CONSTRAINT ON (c:ABContainer) ASSERT c.container_id IS UNIQUE;

// ─────────────────────────────────────────────────────────────────────────────────
// SCHEMA DOCUMENTATION (as comments)
// ─────────────────────────────────────────────────────────────────────────────────

// ABSession Node:
// {
//   session_id: string (unique),
//   project_path: string,
//   task_description: string,
//   start_time: datetime,
//   end_time: datetime (optional),
//   base_branch: string,
//   status: "pending" | "running" | "completed" | "failed"
// }

// ABContainer Node:
// {
//   container_id: string (unique),
//   session_id: string (foreign key),
//   mode: "none" | "semi" | "auto" | "hybrid",
//   branch_name: string,
//   container_name: string,
//   status: string,
//   start_time: datetime,
//   end_time: datetime,
//   exit_code: int,
//   tokens_used: int,
//   tokens_saved: int,
//   execution_time_ms: int,
//   errors: int,
//   files_changed: int,
//   lines_added: int,
//   lines_removed: int,
//   commit_sha: string
// }

// ABMetric Node:
// {
//   metric_type: string,
//   value: float,
//   timestamp: datetime,
//   context: json string
// }

// ABResult Node:
// {
//   session_id: string,
//   mode: string,
//   quality_indicators: json string,
//   branch_name: string
// }

// ─────────────────────────────────────────────────────────────────────────────────
// RELATIONSHIPS
// ─────────────────────────────────────────────────────────────────────────────────

// (ABSession)-[:RUNS]->(ABContainer)
// (ABContainer)-[:MEASURED]->(ABMetric)
// (ABContainer)-[:PRODUCED]->(ABResult)

// ═══════════════════════════════════════════════════════════════════════════════
// ANALYSIS QUERIES
// ═══════════════════════════════════════════════════════════════════════════════

// ─────────────────────────────────────────────────────────────────────────────────
// Query 1: Mode Performance Summary
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (c:ABContainer)
// WHERE c.status = 'completed'
// WITH c.mode AS mode,
//      count(c) AS runs,
//      avg(c.tokens_saved) AS avg_tokens_saved,
//      avg(c.execution_time_ms) AS avg_time_ms,
//      sum(c.errors) AS total_errors,
//      avg(c.files_changed) AS avg_files
// RETURN mode, runs, avg_tokens_saved, avg_time_ms, total_errors, avg_files
// ORDER BY avg_tokens_saved DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 2: Best Mode Recommendation
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (c:ABContainer)
// WHERE c.status = 'completed'
// WITH c.mode AS mode,
//      count(c) AS runs,
//      avg(toFloat(c.tokens_saved)) AS efficiency,
//      avg(toFloat(c.errors)) AS error_rate,
//      avg(toFloat(c.execution_time_ms)) AS speed
// WHERE runs >= 3
// WITH mode, runs, efficiency, error_rate, speed,
//      (coalesce(efficiency, 0) * 0.4) -
//      (coalesce(error_rate, 0) * 100 * 0.3) -
//      (coalesce(speed, 0) / 10000 * 0.3) AS score
// RETURN mode, runs, efficiency, error_rate, speed, score
// ORDER BY score DESC
// LIMIT 1;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 3: Session History
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (s:ABSession)-[:RUNS]->(c:ABContainer)
// WHERE s.status = 'completed'
// RETURN s.session_id, s.task_description, s.start_time,
//        collect({mode: c.mode, tokens_saved: c.tokens_saved, errors: c.errors}) AS containers
// ORDER BY s.start_time DESC
// LIMIT 20;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 4: Mode Comparison for Specific Task Type
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (s:ABSession)-[:RUNS]->(c:ABContainer)
// WHERE s.task_description CONTAINS $task_keyword
//   AND c.status = 'completed'
// WITH c.mode AS mode, avg(c.tokens_saved) AS avg_saved, count(*) AS samples
// RETURN mode, avg_saved, samples
// ORDER BY avg_saved DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 5: Efficiency Over Time (Learning Curve)
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (s:ABSession)-[:RUNS]->(c:ABContainer)
// WHERE c.mode = $mode AND c.status = 'completed'
// RETURN s.start_time AS time, c.tokens_saved AS efficiency
// ORDER BY s.start_time;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 6: Error Analysis by Mode
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (c:ABContainer)
// WHERE c.status = 'completed'
// WITH c.mode AS mode,
//      sum(c.errors) AS total_errors,
//      count(c) AS total_runs,
//      toFloat(sum(c.errors)) / count(c) AS error_rate
// RETURN mode, total_errors, total_runs, error_rate
// ORDER BY error_rate ASC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 7: Branch Comparison (Git Stats)
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (s:ABSession)-[:RUNS]->(c:ABContainer)
// WHERE s.session_id = $session_id
// RETURN c.mode, c.branch_name, c.commit_sha,
//        c.files_changed, c.lines_added, c.lines_removed
// ORDER BY c.mode;

// ─────────────────────────────────────────────────────────────────────────────────
// Query 8: Dynamic Mode Selection (for autonomous system)
// ─────────────────────────────────────────────────────────────────────────────────
// This query is used by the autonomous optimizer to select the best mode
// based on recent performance data
//
// MATCH (c:ABContainer)
// WHERE c.status = 'completed'
//   AND c.start_time > datetime() - duration('P7D')  // Last 7 days
// WITH c.mode AS mode,
//      count(c) AS recent_runs,
//      avg(c.tokens_saved) AS recent_efficiency,
//      avg(c.errors) AS recent_errors
// WHERE recent_runs >= 2
// WITH mode,
//      recent_efficiency * 0.5 - recent_errors * 50 AS dynamic_score
// RETURN mode, dynamic_score
// ORDER BY dynamic_score DESC
// LIMIT 1;

// ═══════════════════════════════════════════════════════════════════════════════
// CLEANUP QUERIES
// ═══════════════════════════════════════════════════════════════════════════════

// Delete old sessions (> 30 days)
// MATCH (s:ABSession)
// WHERE s.start_time < datetime() - duration('P30D')
// DETACH DELETE s;

// Reset all A/B data (DANGEROUS)
// MATCH (n) WHERE n:ABSession OR n:ABContainer OR n:ABMetric OR n:ABResult
// DETACH DELETE n;
