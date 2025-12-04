// ═══════════════════════════════════════════════════════════════════════════════
// PCx MEMGRAPH SCHEMA
// Performance Comparison eXperiment
// ═══════════════════════════════════════════════════════════════════════════════

// ─────────────────────────────────────────────────────────────────────────────────
// INDEXES
// ─────────────────────────────────────────────────────────────────────────────────

CREATE INDEX ON :PCxExperiment(experiment_id);
CREATE INDEX ON :PCxExperiment(workload);
CREATE INDEX ON :PCxResult(mode);
CREATE INDEX ON :PCxResult(experiment_id);
CREATE INDEX ON :PCxPhase(phase_name);

// ─────────────────────────────────────────────────────────────────────────────────
// CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────────────

CREATE CONSTRAINT ON (e:PCxExperiment) ASSERT e.experiment_id IS UNIQUE;

// ─────────────────────────────────────════════════════════════════════════════════
// SCHEMA DOCUMENTATION
// ─────────────────────────────────────────────────────────────────────────────────

// PCxExperiment: Root node for an experiment
// {
//   experiment_id: string,
//   workload: "simple" | "medium" | "complex",
//   start_time: datetime,
//   end_time: datetime,
//   winner: string (mode name),
//   winner_reason: string
// }

// PCxResult: Result for one mode in an experiment
// {
//   experiment_id: string,
//   mode: "none" | "semi" | "auto" | "hybrid",
//   duration_ms: int,
//   tokens_consumed: int,
//   tokens_saved: int,
//   efficiency_ratio: float,
//   context_retention: float,
//   error_recovery: float,
//   lint_score: float
// }

// PCxPhase: Individual phase within a result
// {
//   phase_name: string,
//   duration_ms: int,
//   tokens_before: int,
//   tokens_after: int,
//   errors_encountered: int,
//   errors_resolved: int
// }

// ─────────────────────────────────────────────────────────────────────────────────
// RELATIONSHIPS
// ─────────────────────────────────────────────────────────────────────────────────

// (PCxExperiment)-[:TESTED]->(PCxResult)
// (PCxResult)-[:HAS_PHASE]->(PCxPhase)
// (PCxResult)-[:COMPARED_TO]->(PCxResult)  // Same experiment, different mode

// ═══════════════════════════════════════════════════════════════════════════════
// ANALYSIS QUERIES
// ═══════════════════════════════════════════════════════════════════════════════

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Mode Performance by Workload Complexity
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (e:PCxExperiment)-[:TESTED]->(r:PCxResult)
// WITH e.workload AS workload, r.mode AS mode,
//      avg(r.efficiency_ratio) AS avg_efficiency,
//      avg(r.context_retention) AS avg_retention,
//      count(*) AS samples
// RETURN workload, mode, avg_efficiency, avg_retention, samples
// ORDER BY workload, avg_efficiency DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Winner Distribution
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (e:PCxExperiment)
// WHERE e.winner IS NOT NULL
// WITH e.workload AS workload, e.winner AS winner, count(*) AS wins
// RETURN workload, winner, wins
// ORDER BY workload, wins DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Best Mode by Metric
// ─────────────────────────────────────────────────────────────────────────────────
// // Best for efficiency
// MATCH (r:PCxResult)
// WITH r.mode AS mode, avg(r.efficiency_ratio) AS score
// RETURN "efficiency" AS metric, mode, score
// ORDER BY score DESC LIMIT 1
// UNION
// // Best for retention
// MATCH (r:PCxResult)
// WITH r.mode AS mode, avg(r.context_retention) AS score
// RETURN "retention" AS metric, mode, score
// ORDER BY score DESC LIMIT 1
// UNION
// // Best for error recovery
// MATCH (r:PCxResult)
// WITH r.mode AS mode, avg(r.error_recovery) AS score
// RETURN "recovery" AS metric, mode, score
// ORDER BY score DESC LIMIT 1;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Efficiency Trend Over Time
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (e:PCxExperiment)-[:TESTED]->(r:PCxResult)
// WHERE r.mode = $mode
// RETURN e.start_time AS time, r.efficiency_ratio AS efficiency
// ORDER BY e.start_time;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Mode Comparison Matrix
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (e:PCxExperiment)-[:TESTED]->(r1:PCxResult),
//       (e)-[:TESTED]->(r2:PCxResult)
// WHERE r1.mode < r2.mode
// WITH r1.mode AS mode_a, r2.mode AS mode_b,
//      avg(r1.efficiency_ratio - r2.efficiency_ratio) AS efficiency_diff,
//      count(*) AS comparisons
// RETURN mode_a, mode_b, efficiency_diff, comparisons
// ORDER BY abs(efficiency_diff) DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Dynamic Mode Recommendation
// ─────────────────────────────────────────────────────────────────────────────────
// Based on recent experiments, recommend the best mode for a given workload
//
// MATCH (e:PCxExperiment)-[:TESTED]->(r:PCxResult)
// WHERE e.workload = $workload
//   AND e.start_time > datetime() - duration('P7D')  // Last 7 days
// WITH r.mode AS mode,
//      avg(r.efficiency_ratio) * 0.3 +
//      avg(r.context_retention) * 0.3 +
//      avg(r.error_recovery) * 0.4 AS composite_score,
//      count(*) AS recent_samples
// WHERE recent_samples >= 2
// RETURN mode, composite_score
// ORDER BY composite_score DESC
// LIMIT 1;

// ═══════════════════════════════════════════════════════════════════════════════
// STATISTICAL ANALYSIS
// ═══════════════════════════════════════════════════════════════════════════════

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Statistical Summary per Mode
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (r:PCxResult)
// WITH r.mode AS mode,
//      count(r) AS n,
//      avg(r.efficiency_ratio) AS mean_eff,
//      stDev(r.efficiency_ratio) AS std_eff,
//      min(r.efficiency_ratio) AS min_eff,
//      max(r.efficiency_ratio) AS max_eff
// RETURN mode, n, mean_eff, std_eff, min_eff, max_eff
// ORDER BY mean_eff DESC;

// ─────────────────────────────────────────────────────────────────────────────────
// Query: Significant Differences (>10% improvement)
// ─────────────────────────────────────────────────────────────────────────────────
// MATCH (e:PCxExperiment)-[:TESTED]->(r_base:PCxResult {mode: 'none'}),
//       (e)-[:TESTED]->(r_test:PCxResult)
// WHERE r_test.mode <> 'none'
// WITH r_test.mode AS mode,
//      avg(r_test.efficiency_ratio - r_base.efficiency_ratio) AS improvement,
//      count(*) AS samples
// WHERE abs(improvement) > 0.1  // >10% difference
// RETURN mode, improvement, samples,
//        CASE WHEN improvement > 0 THEN 'BETTER' ELSE 'WORSE' END AS verdict
// ORDER BY improvement DESC;

// ═══════════════════════════════════════════════════════════════════════════════
// CLEANUP
// ═══════════════════════════════════════════════════════════════════════════════

// Delete old experiments (>30 days)
// MATCH (e:PCxExperiment)
// WHERE e.start_time < datetime() - duration('P30D')
// DETACH DELETE e;

// Reset all PCx data
// MATCH (n) WHERE n:PCxExperiment OR n:PCxResult OR n:PCxPhase
// DETACH DELETE n;
