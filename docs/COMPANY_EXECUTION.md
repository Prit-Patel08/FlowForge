# FlowForge Company Execution Playbook

Date: 2026-02-20  
Owner: Founder + Engineering  
Status: Active operating document

## Purpose

This file defines how FlowForge is built as a company-level product, not a demo project.
From now on, implementation follows a strict rule:

1. Discuss idea first.
2. Decide scope and success metric.
3. Implement only if it improves company outcomes.

No feature work starts without a written decision.

## Company Goal

Build the most trusted local-first execution guardrail for AI and automation workloads.

## Strategic Wedge (Current)

Primary user:
- Engineers running long local scripts/agents where runaway behavior causes real cost, failure, or machine instability.

Primary promise:
- Detect runaway behavior quickly, intervene safely, and explain exactly why.

Primary proof in first 60 seconds:
- Install works.
- Demo shows detection + recovery.
- Timeline shows reason and confidence.

## Execution Filters (Must Pass)

Every task is scored before implementation:

1. 60-second value clarity
- Does this make value obvious fast?

2. Trust and reliability
- Does this reduce incidents, false positives, or recovery risk?

3. Adoption and distribution
- Does this make onboarding, deployment, or integration easier?

4. Complexity discipline
- Is this low-complexity relative to impact?

Policy:
- Implement only if score is at least 3/4.
- If 2/4 or less, defer or delete.

## Idea-First Workflow (Mandatory)

No direct implementation from raw ideas. Use this sequence:

1. Idea Brief (max 15 lines)
- Problem
- Target user
- Why now
- Expected outcome
- Metric moved
- Risks

2. Alignment Check
- Map to at least one blueprint domain.
- Confirm it does not violate scope freeze.

3. Decision
- `Approve`
- `Defer`
- `Reject`

4. Implementation Plan (only after approve)
- Scope in/out
- File list
- Test plan
- Rollback plan

5. Build + Validate
- Code
- Tests
- CI gates
- Docs update

6. Post-merge Verification
- Confirm metric movement or note no change.

## Scope Freeze (Current Operating Window)

Keep in active scope:

1. Runtime guard reliability
- process supervision correctness
- safe teardown/restart behavior
- decision transparency

2. Evidence and trust
- timeline quality
- audit/event integrity
- reason/confidence clarity

3. Onboarding and first value
- one-command setup
- demo reliability
- dashboard clarity for incidents

4. Production readiness baseline
- CI reliability
- Docker hardening
- security defaults and docs

Defer for now:

1. Multi-tenant SaaS layers
2. Billing/commercial packaging
3. Broad marketplace integrations
4. Advanced agent-runtime platformization beyond current reliability needs
5. New UI surfaces without direct pilot demand

## 12-Week Execution Plan (Company-Optimized)

## Weeks 1-2: Product Truth and Reliability Foundation

1. Lock deterministic runtime behavior
- Validate process-group teardown across fixtures
- Reduce flaky behavior in race/stress contexts
- Publish known limits clearly

2. Stabilize detection trust
- False-positive guard tuning
- Confidence explanation quality checks
- Baseline drift checks against fixture corpus

3. Onboarding hardening
- Ensure install/demo path is deterministic
- Validate first-value report quality

Exit criteria:
- Green CI on all required checks for 7 consecutive days
- Demo success rate >95% across clean local runs

## Weeks 3-4: Explainability and Evidence Quality

1. Timeline and incident story quality
- Every intervention has reason, actor, confidence, and timestamp

2. Event model cleanup
- Reduce overlapping/duplicate event concepts
- Document canonical event envelope

3. Operator trust docs
- Tight runbook for common failures
- Clear escalation and rollback steps

Exit criteria:
- Operator can answer "what happened and why" in under 2 minutes

## Weeks 5-6: Real Pilot Loop

1. Run controlled pilots (3-5 users)
- Measure setup time
- Measure false positives
- Measure retained usage after first week

2. Prioritize by pain, not ideas
- Highest pain + highest frequency first

3. Remove dead weight
- Delete features/docs not used by pilot users

Exit criteria:
- At least 2 users keep it enabled on real workloads

## Weeks 7-8: Operational Maturity

1. Reliability ritual
- Weekly SLO review
- Error-budget policy usage
- Recovery drill cadence

2. Security and trust hygiene
- Vulnerability process discipline
- Disclosure and response playbook checks

3. Release governance
- Freeze/canary/soak/rollback rhythm

Exit criteria:
- Two clean release cycles with no emergency rollback

## Weeks 9-12: Distribution and Repeatability

1. Integration surface (high signal only)
- Keep only integrations that reduce adoption friction materially

2. Packaging and messaging
- Product narrative centered on trust + deterministic control

3. Evidence-backed positioning
- Publish measurable claims only

Exit criteria:
- Repeatable onboarding + repeatable pilot conversion

## Weekly Operating Cadence

Monday:
1. Review metrics and incidents.
2. Select top 3 tasks by impact.
3. Freeze anything not passing 3/4 filter.

Tuesday-Thursday:
1. Execute approved tasks only.
2. Keep PRs small and test-backed.
3. Update docs with each behavior change.

Friday:
1. Run release checkpoint.
2. Review what moved metrics.
3. Remove one low-value item from backlog.

## Required Metrics

Track these every week:

1. Time-to-first-value
- `install start -> demo summary shown`

2. Detection latency
- seconds from runaway onset to intervention

3. False-positive rate
- interventions later classified as unnecessary

4. Recovery success rate
- intervention followed by healthy continuation/restart

5. Reliability gate pass rate
- percent of successful CI runs on `main`

6. User retention proxy (pilot phase)
- number of users still running FlowForge after 7 days

## Engineering Rules

1. No implementation before approval.
2. No large mixed PRs (feature + refactor + docs unrelated).
3. Any behavior change requires:
- tests
- docs update
- rollback note

4. Remove stale files quickly.
5. Keep one canonical path for each concern (no duplicate build specs/docs roots).

## “Do Not Build Now” List

1. Fancy dashboards without reliability gains
2. Generic AI assistant features with unclear trust value
3. Broad plugin ecosystem before core runtime is stable
4. Any enterprise checkbox without pilot pull
5. Deep architecture rewrites without metric evidence

## Fast Decision Templates

Use this format for each new idea:

```text
Idea:
Problem:
Who is blocked:
Current workaround:
Expected outcome:
Metric moved:
Complexity estimate (S/M/L):
Score (value/trust/adoption/complexity): X/4
Decision: Approve / Defer / Reject
```

## Merge Discipline

1. Feature branches by default.
2. Merge only when CI is green.
3. If emergency direct merge is needed, log reason in commit body and follow with corrective PR.

## Alignment with Blueprint

This playbook operationalizes the bigger company blueprint by sequencing work:

1. First: runtime trust and explainability
2. Second: governance and reliability rituals
3. Third: distribution and optional expansion

This order is mandatory.  
If priorities conflict, choose trust and reliability first.
