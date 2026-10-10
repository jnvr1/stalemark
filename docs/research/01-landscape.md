# Documentation Drift Detection: Landscape Research

Date of research: 2026-10-10. GitHub stats (stars, license, language, last push) were pulled live via the GitHub API on that date. Anything I could not verify is marked "unverified".

---

## 0. Headline correction to the brief

The brief says the borghei skill has "~451 stars". The live GitHub API shows **891 stars** on the *whole* repo (`borghei/Claude-Skills`), last pushed 2026-10-07 ([repo](https://github.com/borghei/Claude-Skills)). Those stars belong to a ~hundreds-of-skills monorepo, not to the doc-drift-detector skill. The skill itself is one folder (`engineering/doc-drift-detector`) last touched 2026-06-22 (GitHub commits API). Star count is therefore a poor demand signal for this specific capability.

---

## 1. borghei/claude-skills "doc-drift-detector"

- Source: [SKILL.md](https://github.com/borghei/Claude-Skills/blob/main/engineering/doc-drift-detector/SKILL.md), [repo](https://github.com/borghei/Claude-Skills)
- **What it does:** four stdlib-only Python scripts: `drift_analyzer.py` (maps code dirs to docs, compares git modification histories, detects renamed files, version drift, broken refs), `doc_staleness_scorer.py` (weighted 0-100 freshness score), `api_doc_validator.py` (Python AST signatures vs markdown API docs), `link_checker.py` (local links, anchors, images, case sensitivity, optional external HEAD checks).
- **License:** "MIT + Commons Clause" (declared in the SKILL.md front matter; GitHub reports `NOASSERTION`). Commons Clause forbids selling the software, so it is **not OSI open source**. This blocks reuse/forking into a commercial or standard OSS project.
- **Language:** Python 3.8+, stdlib only. GitHub labels the repo "HTML" because of the generated `site/` pages.
- **Activity:** repo pushed 2026-10-07; skill folder last changed 2026-06-22; SKILL.md says v2.1.0, updated 2026-06-17.
- **CI:** yes, by script convention. All tools have `--json`, and I read the code: `drift_analyzer.py` exits 2 on bad input and 1 when "serious" drift is found; `link_checker.py` exits 1 on broken links or duplicate anchors; `doc_staleness_scorer.py` exits 1 below `--threshold`; `api_doc_validator.py` exits 1 on high-severity findings. There is **no packaged GitHub Action**: the SKILL.md references "GitHub Actions and pre-commit recipes" in a docs file, i.e. copy/paste YAML ([workflows reference](https://github.com/borghei/Claude-Skills/blob/main/engineering/doc-drift-detector/references/workflows-and-tool-reference.md) listed in the tree).
- **Deterministic vs LLM:** deterministic scripts, wrapped in an LLM-agent skill prompt.
- **Limitations (from its own "Does NOT cover"):** AST validation is **Python only** (no TS/Go/Rust/Java); no external uptime monitoring; no automatic rewriting (issues only tagged AUTO/SEMI/MANUAL); no content-quality assessment. Its core "drift" signal is git-timestamp comparison between mapped code and doc files, a heuristic that says "code is newer than doc", not "doc is wrong".
- **Distribution:** is a *skill* (instructions for an agent), not a standalone product with releases/package registry.

## 2. DriftGuard (lablab.ai hackathon)

There are **multiple unrelated "DriftGuard" projects**; the brief's matches the first:

| Project | Notes | Source |
|---|---|---|
| Team Said, IBM Bob Hackathon (lablab.ai) | User pastes a public GitHub URL; backend pulls diff + docs via GitHub API; LangGraph pipeline with Groq LLM (via LangChain ChatGroq) flags stale sections with severity and suggested fix; 0-100 health score; Quick/PR/Manual scan modes; FastAPI + React. **Page mentions no CI features and no limitations.** | [lablab page](https://lablab.ai/ai-hackathons/ibm-bob-hackathon/team-said/driftguard-ai-doc-drift-detection-engine) |
| Its repo `saidibnerradi603/driftguard-ai` | 0 stars, Python, no license in GitHub metadata, last push 2026-07-13 | GitHub API |
| `vamshimohan-777/DriftGuard` | Hackathon prototype comparing Markdown ADRs, OpenAPI specs, README vs source; Node.js; writes drift report + HTML dashboard. 0 stars, no license, pushed 2026-09-26 | [repo](https://github.com/vamshimohan-777/DriftGuard) |
| `mahimathacker/driftguard` | Solidity/TS SDK/docs cross-layer, deterministic, no AI. 9 stars, MIT, TypeScript, pushed 2026-06-10 | [repo](https://github.com/mahimathacker/driftguard) |
| `drishti55/DriftGuard` | Local LLM + RAG cross-artifact consistency. 1 star | [repo](https://github.com/drishti55/DriftGuard) |

Assessment: a SaaS-style demo (web UI, GitHub API, LLM) with no exit codes or Action; non-deterministic; no license; effectively a prototype. Not a competitor for a CI tool.

## 3. The skill-registry tools

| Skill | What it does | Evidence |
|---|---|---|
| **doc-freshness** (joaquimscosta/arkhe-claude-plugins, path `plugins/doc/skills/doc-freshness`) | Claude skill; python scripts (`scan_freshness.py`, `link_checker.py`, `version_checker.py`, `claude_md_checker.py`) do mechanical checks: broken links, backtick path verification, git-age staleness, frontmatter version drift. Three drift types: code-doc, cross-doc, stale references. Report-only, no auto-fix. `setup` mode scaffolds a workflow that uses `joaquimscosta/docs-health-action@v1`. Repo: 21 stars, MIT, Python, last push 2026-08-14; the doc plugin last changed 2026-07-29. The action repo has 0 stars, MIT, pushed 2026-06-26. Code-doc "drift" judgments are made by the LLM agent using Grep/Read (so non-deterministic for that tier). | [SKILL.md](https://github.com/joaquimscosta/arkhe-claude-plugins/blob/main/plugins/doc/skills/doc-freshness/SKILL.md), [repo](https://github.com/joaquimscosta/arkhe-claude-plugins), [listing](https://skillselion.com/skills/joaquimscosta/arkhe-claude-plugins/doc-freshness), [action repo](https://github.com/joaquimscosta/docs-health-action) |
| **readme-doc-drift-detector** | Could not locate a source repo. Only surfaced on an unusual university sandbox page (v0.1.0): compares README commands to `package.json` scripts, outputs a markdown report, labels itself "static analysis, manual review required". Provenance unverified. | [page](https://ssk16-ars-flow.sandbox.dsrs.illinois.edu/skills/readme-doc-drift-detector) |
| **doc-maintenance** (paperclipai/paperclip, `.agents/skills/doc-maintenance`) | Pure agent prompt: reads `git log` since a `.doc-review-cursor` SHA file, classifies commits (feature/breaking), audits README.md, doc/SPEC.md, doc/PRODUCT.md and opens a minimal fix PR. No scripts, no exit codes; LLM-driven; project-specific paths. Parent repo has ~99.6k stars (MIT, TypeScript), but that is the product, not this skill. Skill last changed 2026-07-10. | [SKILL.md](https://github.com/paperclipai/paperclip/blob/master/.agents/skills/doc-maintenance/SKILL.md), [listing](https://skills.sh/paperclipai/paperclip/doc-maintenance) |

Common limitation: these are agent *instructions*, not CI-native tools. They need an LLM runtime in the loop and give no stable exit-code contract.

## 3b. Additional OSS doc-drift tools discovered (not in the brief, and they matter most)

These are the true direct competitors. All are tiny/new, which is itself a finding.

| Tool | What it does | License / lang / stars / last push | Deterministic? | CI |
|---|---|---|---|---|
| [pallaprolus/drift](https://github.com/pallaprolus/drift) (`npx docs-drift`) | Pairs JSDoc/docstrings/README code blocks with code anchors; detects param mismatches, return type drift, renamed symbols, stale signatures, wrong arg counts, docs older than code (git blame). Langs: TS/JS, Python, Go, Rust, Java. VS Code extension + CLI + Action. | MIT, TypeScript, 2 stars, pushed 2026-10-08 | Yes by default; AI check opt-in, on demand, editor only | Action `uses: pallaprolus/drift@v0`, fails job at `fail-on` severity, job summary |
| [georg-nikola/docs-drift](https://github.com/georg-nikola/docs-drift) | Executes code blocks in Markdown (JS, Python, Bash, Go, Ruby), `--changed-only`, JSON/HTML reports | MIT, Go, 0 stars, pushed 2026-09-28 | Yes | CLI + GitHub Action |
| [zk-hypersolid/doc-drift-check](https://github.com/zk-hypersolid/doc-drift-check) | PR Action: verifies each documented claim against code before and after the change, reports only claims that flipped | MIT, Python, 0 stars, pushed 2026-09-21 | No: requires a paid third-party API key (typesafe.ai), `fail-on-finding` defaults to false | Action, PR comment |
| [jbrockSTL/doc-drift](https://github.com/jbrockSTL/doc-drift) | LLM reads PR diff vs docs, posts PR comment, can fail build on high-confidence drift | no license, 0 stars, pushed 2026-01-25 | No | Action |
| [tayfuryldz/driftcheck](https://github.com/tayfuryldz/driftcheck) | Version drift between README and toolchain files (rust-toolchain.toml, CI tags) | NOASSERTION, 0 stars | Yes | `--git-mode`, step summary |
| [agent-sh/sync-docs](https://github.com/agent-sh/sync-docs) | Agent: finds outdated refs, updates CHANGELOG, flags stale examples | 3 stars, JS, pushed 2026-10-06 | Mixed | n/a |
| [wesleytanws/DOCER](https://github.com/wesleytanws/DOCER) | Academic: outdated *code element references* in README/wiki, comparing two revisions; shipped as a GitHub Action; `.DOCER_exclude` for false positives | MIT, 4 stars, last push 2023-05-28 (dormant) | Yes | Action (per [paper](https://arxiv.org/abs/2307.04291v1)) |
| [machine-operable/readme-ci](https://github.com/machine-operable/readme-ci) | Runs README code examples in a sandbox, pass/fail for CI (from PyPI) | stats unverified | Yes | CLI |

Source for the group listing: [search results](https://github.com/pallaprolus/drift), and per-repo GitHub API stats.

---

## 4. Commercial players

| Vendor | What it does about drift | Can OSS replicate? | Sources |
|---|---|---|---|
| **Swimm** | Docs embed code snippets, "smart tokens", "smart paths". A CI/PR check compares docs to the PR's code changes, classifying docs as up to date / out of sync (auto-fixable, committed to the PR) / outdated (needs human). CI exits non-zero when attention is needed. Needs full git history (no shallow clones). Pricing not public (reported as custom/LOC-based by a third-party review). Proprietary. | The coupling-by-anchor idea (doc references code location, verify on PR) is replicable OSS with deterministic AST/git; the proprietary "patented auto-sync" algorithm is not. | [Swimm CI blog](https://swimm.io/blog/continuous-documentation-through-continuous-integration-with-swimm), [VS Code docs](https://docs.swimm.io/ide-integrations/vs-code/ide-plugins-vscode-keep-docs-up-to-date), [review](https://tooliverse.ai/tools/swimm) |
| **Mintlify** | Agent drafts docs from code changes and opens PRs ("Update from code changes" automation; scheduled or per source PR). "Autopilot" GitHub check on source PRs (Custom plans). Agent needs Pro/Enterprise. Docs say AI output needs human review. Ships Vale as a built-in CI check. | The *detect* step is replicable; Mintlify's value is the generate/PR step plus hosting. | [automations](https://mintlify.com/docs/automations/reference), [agent](https://www.mintlify.com/docs/agent/index), [blog](https://www.mintlify.com/blog/workflows-usecases) |
| **GitBook** | GitBook Agent reviews change requests and proposes doc improvements as change requests; Git Sync links docs repo to GitHub/GitLab. Does not verify docs against source code. Proactive alignment appears to be roadmap per sources (unverified). | Detection against code: not offered, so open gap. | [Agent](https://www.gitbook.com/blog/introducing-docs-agent), [automatic suggestions](https://gitbook.com/docs/gitbook-agent/automatic-documentation-suggestions) |
| **ReadMe.com** | `rdme` CLI/Action syncs OpenAPI definitions into ReadMe, and validates/analyzes the spec. No doc-vs-code drift feature found (searching for "Docs Audit"/"API Drift" returned nothing). | The spec sync is trivial; no drift capability to copy. | [rdme Action](https://docs.readme.com/docs/github-actions-openapi-example), [rdme](https://github.com/readmeio/rdme) |
| **DeepDocs** | GitHub app: listens to commits, detects drift, opens a branch with updates; Docusaurus/MkDocs aware. Proprietary freemium; free-tier details conflict between third-party sources. | Directly OSS-replicable in concept. | [Show HN](https://news.ycombinator.com/item?id=44582890), [AlternativeTo](https://alternativeto.net/software/deepdocs/about) |
| **Doc Detective** | OSS (see section 5). | n/a | below |
| **CodeRabbit** | `@coderabbitai generate docstrings` (beta, Pro plan) fills missing/incomplete docstrings via ast-grep, delivered as a follow-up PR. It *generates*, it does not detect stale prose docs. | Generation: needs an LLM. Detection of stale docstring params: deterministic. | [docs](https://docs.coderabbit.ai/finishing-touches/docstrings) |
| **GitHub Copilot** | No dedicated docs-drift feature found. GitHub's agent card lists "create or update documentation" as a generic cloud-agent task; community shows custom agentic workflows doing PR doc checks. | n/a | [GitHub docs](https://docs.github.com/en/copilot/responsible-use/agents), [community example](https://github.com/TechWatching/ghcopilotdevdays2026/pull/6) |

Takeaway: commercial effort is overwhelmingly **generate-the-fix with an LLM**, bundled with a hosting platform. What is *cheap and open* is the **detect-and-gate** step.

---

## 5. Adjacent OSS tools (one slice each)

| Tool | Slice | License / lang / stars / last push | Notes |
|---|---|---|---|
| [lychee](https://github.com/lycheeverse/lychee) | Broken links/mail in Markdown/HTML; Action `lycheeverse/lychee-action` | Apache-2.0, Rust, ~4.0k, 2026-10-05 | Exit codes: 0 ok, 1 runtime/input error, 2 link failures, 3 config error. Fragment checks opt-in and limited; no JS rendering. |
| [markdown-link-check](https://github.com/tcort/markdown-link-check) | Links | ISC, JS, ~718, 2026-07-28 | |
| [Vale](https://github.com/vale-cli/vale) | Prose style; also lints comments/docstrings | MIT, Go, ~6.2k, 2026-10-06 | Checks style, not technical correctness. |
| [Doc Detective](https://github.com/doc-detective/doc-detective) | Executes documented UI/API/shell procedures in a browser, JSON PASS/FAIL | **AGPL-3.0**, TypeScript, ~136, 2026-10-02 | CI via `npx` (and a GH Action per [CI guide](https://mintlify.com/doc-detective/doc-detective/guides/ci-cd)); no reporting/notifications built in; heavy browser installs; AGPL deters embedding. |
| [byexample](https://pypi.org/project/byexample/7.4.6/), [markdown-doctest](https://github.com/Widdershin/markdown-doctest), [phmdoctest](https://pypi.python.org/pypi/phmdoctest) | Run fenced code blocks in README as tests | markdown-doctest: MIT, ~170 stars, last push 2020 (dormant) | Only checks "does not crash", per its own README text. I could not verify mdsh/cram details. |
| [oasdiff](https://github.com/oasdiff/oasdiff) | OpenAPI **spec vs spec** breaking-change diff | Apache-2.0, Go, ~1.4k, 2026-10-09 | Action `oasdiff-action`; works on spec documents, not source code. |
| [openapi-diff](https://github.com/OpenAPITools/openapi-diff) | Same, Java | Apache-2.0, ~1.1k, 2026-09-04 | |
| [Optic](https://github.com/opticdev/optic) | OpenAPI diff/lint/test | MIT, TS, ~1.5k, **archived**, last push 2026-01-08 | Acquired by Atlassian in 2024 ([Atlassian](https://www.atlassian.com/blog/announcements/optic-acquisition)). |
| [Spectral](https://github.com/stoplightio/spectral) | OpenAPI/JSON linting | Apache-2.0, TS, ~3.2k, 2026-10-02 | Lint rules, not code comparison. |
| [typedoc](https://github.com/TypeStrong/typedoc) | API docs from TS; has validation options (not verified in this session: fetch of the validation doc returned 404) | Apache-2.0, ~8.4k, 2026-07-13 | Generated docs cannot drift in signature, but hand-written prose can. |

None of these cross-checks **hand-written prose/README claims vs code**. They either check form (links, style), execute snippets, or diff *two specs* against each other.

IaC "drift" (Terraform, Kubernetes GitOps) is naming-inspiration only; "DriftGuard" is already overloaded (four unrelated repos in the search results above).

---

## 6. Research literature

### 6.1 arXiv 2603.00489: "Does My README File Need To Be Updated? Exploring LLM-Based README Maintenance"
Source: [arXiv HTML](https://arxiv.org/html/2603.00489v1). Authors: Gao, Lin, Treude, Gay, Zahedi (28 Feb 2026).

- **Method:** PR-level human-in-the-loop pipeline of 5 components: relevance classifier (high-recall filter on PR metadata), context-sufficiency analyser, context retrieval (MiniLM cosine similarity over file patches), section localisation + generation (top-5 README section indices with justification), QA reviewer to filter generic/hallucinated output. Two variants: static linear vs agentic (retrieval loop + self-reflection loop). Three open-weight models at temperature 0: Llama-3.3-70B, Qwen-3-32B, Gemma-3-27B.
- **Dataset:** built from the Lin et al. corpus (826 repos, ~4.4M PRs); positives = PRs that edited root README alongside code, negatives = random PRs without README edits; 714 repos after filtering (the paper's counts reportedly do not fully reconcile).
- **Metrics:** entry recall, specificity, "user-facing accuracy" (precision simulated at 1:99 prevalence), index recall, MRR, hierarchical recall L1-L4.
- **Results:** best config (Qwen-3-32B agentic): entry recall 0.52, specificity 0.99, user-facing accuracy 0.29, index recall 0.58, MRR 0.64. Gemma agentic had higher recall (0.69) but only 0.10 user-facing accuracy. Hierarchical recall for Qwen agentic: 0.96 (L1), 0.85 (L2). Paper's own summary: roughly **1 valid recommendation per 4 shown**.
- **Retrospective study:** 20 repos, 1,832 flagged PRs, 425 after a one-month filter; 163 manually annotated, 21.5% valid. **Maintainer case study (JabRef):** 6 of 8 recommendations valid.
- **What worked:** agentic loop raised specificity; selective context beat dumping all patches (full context caused format failures in 17.3-27% of cases); section-level localisation is reliable even when exact paragraph is missed.
- **What failed:** exact index recall stayed ~0.55-0.60; QA reviewer weak on negatives for Gemma/Qwen; ~40%+ of "failures" were noisy ground truth (cosmetic edits, tangled PRs, retroactive fixes), ~1/3 missing context, ~1/4 structural ambiguity.
- **Code:** replication package at [github.com/Haoyu-Gao/README-auto-update](https://github.com/Haoyu-Gao/README-auto-update) (0 stars, no license in metadata, Python; link taken from the paper HTML). The authors suggest a CI documentation bot posting paragraph-level PR comments, i.e. the product idea is the paper's own future-work.

### 6.2 Other papers
- **DocPrism** (arXiv [2511.00215](https://arxiv.org/abs/2511.00215), ISSTA 2026): LLM-based code-vs-doc incorrectness detection across Python/TS/C++/Java. Naive prompting flags 98% of functions; their Local Categorization + External Filtering cuts that to 14% and lifts F1 from 0.22 to 0.77; 17% flagged at 0.63 precision; authors estimate at least 11% of code-doc pairs are inconsistent. No code link on the abstract page. Lesson: raw LLM judging is hopelessly noisy without filtering.
- **CASCADE** (arXiv [2604.19400](https://arxiv.org/abs/2604.19400), FSE 2026): generates unit tests from docs and code from docs; reports inconsistency only when existing code fails a test that doc-derived code passes. 71 inconsistent / 814 consistent Java pairs; found 13 new inconsistencies in Java/C#/Rust repos, 10 fixed. Code: [TobiasKiecker/CASCADE](https://github.com/TobiasKiecker/CASCADE) (MIT, 6 stars). Scope is function-level doc comments, not READMEs.
- **DOCER** (Tan, Wagner, Treude; [arXiv 2212.01479](https://arxiv.org/abs/2212.01479) and the tool paper [2307.04291](https://arxiv.org/abs/2307.04291v1)): deterministic detection of outdated code-element references in READMEs/wikis by comparing revisions; over 25% of the 1,000 most popular GitHub projects had at least one outdated reference (per 2307.04291 abstract); shipped as a GitHub Action, now dormant (last push 2023).
- Also surfaced, not read in depth: METAMON ([arXiv 2502.02794](https://arxiv.org/pdf/2502.02794)), CodeFuse-CommitEval ([arXiv 2511.19875](https://arxiv.org/pdf/2511.19875)).

---

## 7. Evidence of demand

1. **GitHub Open Source Survey 2017:** "Incomplete or outdated documentation is a pervasive problem, observed by 93% of respondents." ([opensourcesurvey.org/2017](https://opensourcesurvey.org/2017/)). Old, but the most-cited number.
2. **Measured prevalence:** more than a quarter of the 1,000 most popular GitHub projects contain at least one outdated code reference ([arXiv 2307.04291](https://arxiv.org/abs/2307.04291v1)); at least 11% of code-doc pairs are inconsistent ([DocPrism](https://arxiv.org/abs/2511.00215)).
3. **Hacker News, Show HN "Fix your outdated GitHub docs on autopilot" (DeepDocs):** the author writes "In my 15+ years as a developer, one of the most annoying things I've consistently faced is outdated documentation." ([HN 44712111](https://news.ycombinator.com/item?id=44712111), and [HN 44582890](https://news.ycombinator.com/item?id=44582890), which had 2 points and 1 comment). Weak signal: self-reported by the vendor, almost no engagement.
4. **GitHub issues in unrelated repos independently asking for a CI drift gate** (a pattern, not one big thread):
   - "Docs drift check: fail CI when routes, env vars or CLI flags go undocumented" ([sumisura #199](https://github.com/gio-del/sumisura/issues/199))
   - "feat: enforce documentation drift checks in CI" ([thiscloud-ui #49](https://github.com/mdesantis1984/thiscloud-ui/issues/49))
   - "Add evidence-based documentation drift detection" ([DocTrail #5](https://github.com/iovargasjeff/DocTrail/issues/5))
   - "Documentation drift checks for changes and releases" ([iuliandita/skills #258](https://github.com/iuliandita/skills/issues/258))
   - "docs: fail CI when README.adoc is stale against README_source.adoc" ([tilegroxy #1049](https://github.com/Michad/tilegroxy/issues/1049))
   These are small hobby repos; they show the want is real but each hand-rolls a bespoke script (e.g. [statim PR #36](https://github.com/BEKO2210/statim/pull/36)).
5. **Vendor and community response:** Mintlify, GitBook, Swimm and DeepDocs all ship or pitch doc-currency features (section 4), and at least 5 new OSS repos appeared in 2026 (section 3b). Caveat: I did **not** find Reddit threads (search returned none) or a recent large survey; demand evidence is mostly supply-side and anecdotal.

---

## 8. Comparison table

| Tool | License | Lang | Stars / last push | Deterministic? | CI exit codes / Action | Scope | Key limitation |
|---|---|---|---|---|---|---|---|
| borghei doc-drift-detector | MIT + Commons Clause (not OSI) | Python stdlib | 891 (monorepo) / 2026-10-07 | Yes (scripts) | exit 1/2; no packaged Action | git-age, Python AST API docs, links | Python-only AST; timestamp heuristic; restrictive license |
| DriftGuard (lablab) | none | Python, FastAPI, LangGraph, Groq | 0 / 2026-07-13 | No (LLM) | none stated | web UI; diff + docs via GitHub API | prototype, no CI, no license |
| arkhe doc-freshness | MIT | Python + agent | 21 / 2026-08-14 | Partly | separate `docs-health-action` (0 stars) | links, versions, staleness; LLM for code-doc | needs Claude agent for the deep tier |
| paperclip doc-maintenance | MIT (repo) | prompt only | repo 99.6k / skill 2026-07-10 | No | none | 3 hardcoded docs | project-specific, no tooling |
| pallaprolus/drift | MIT | TS | 2 / 2026-10-08 | Yes (AI opt-in) | Action, `fail-on` | docstrings + README code blocks; 5 langs | tiny, new, signature-level only |
| docs-drift (georg-nikola) | MIT | Go | 0 / 2026-09-28 | Yes | CLI + Action | executes code blocks | only checks that code runs |
| doc-drift-check | MIT | Python | 0 / 2026-09-21 | No (paid API) | Action, off by default | claims flipped by PR | external paid service |
| DOCER | MIT | Python | 4 / 2023-05-28 | Yes | Action | code-element refs in README/wiki | dormant; GitHub-hosted only |
| Swimm | proprietary | n/a | n/a | Mostly (anchors) | non-zero on stale | coupled docs | proprietary, full history needed |
| Mintlify Agent/Autopilot | proprietary | n/a | n/a | No (LLM) | PR check (Custom plan) | generate PRs | paid plans, needs review |
| Doc Detective | AGPL-3.0 | TS | 136 / 2026-10-02 | Yes | JSON results; CI via npx/Action | execute UI/API steps | heavy; AGPL; no built-in reporting |
| lychee | Apache-2.0 | Rust | ~4.0k / 2026-10-05 | Yes | 0/1/2/3 + Action | links only | slice only |
| Vale | MIT | Go | ~6.2k / 2026-10-06 | Yes | CI usable | prose style | no semantic correctness |
| oasdiff | Apache-2.0 | Go | ~1.4k / 2026-10-09 | Yes | Action | spec vs spec | not code vs docs |

---

## 9. Gaps nobody covers

1. **A mature, language-agnostic, deterministic CI tool for hand-written prose claims vs code.** Existing deterministic tools are either Python-only (borghei), signature-level (pallaprolus/drift), or only check that snippets run. Nobody extracts and verifies README claims: CLI flags, env vars, config keys, routes, file paths, package names, version numbers, scripts. (Issues like sumisura #199 ask exactly for routes/env vars/CLI flags.)
2. **Documentation-to-code linkage for ADRs/specs/onboarding guides.** All real tooling targets README and docstrings. ADRs, specs, and onboarding guides appear only in prototypes (vamshimohan-777/DriftGuard) or inside Swimm's proprietary format.
3. **A trustworthy "PR changed something a doc depends on" signal with a stable exit-code contract.** Swimm does this proprietary; OSS equivalents are 0-2 star repos. The one OSS tool built on this idea (DOCER) is dormant.
4. **Precision-first design.** The literature shows LLM-only detection is noisy: roughly 1 valid in 4 (README paper), 98% over-flagging without filters (DocPrism). A tool that uses LLMs only as an optional, gated second stage after deterministic evidence is not offered by anyone with real adoption.
5. **Config-driven doc-to-code mapping** (e.g. "docs/api.md covers `src/api/**`") with suppressions and baselines so existing drift does not block CI day one (only doc-drift-check does the "only what this PR broke" idea, and it needs a paid API).
6. **Open licensing.** The most-starred relevant item carries a Commons Clause; Doc Detective is AGPL; Swimm/Mintlify/DeepDocs are proprietary.
7. **Unified report across slices** (links + versions + claims + snippets + API signatures) rather than gluing lychee + Vale + doc-detective + oasdiff.

## 10. Risks for a new entrant

1. **Crowded at the bottom, not at the top.** At least 7 young OSS repos already target this (section 3b); pallaprolus/drift already ships CLI + Action + 5 languages under MIT. Differentiation must be technical (claim extraction, ADR/spec scope, baselines), not "first".
2. **Zero traction in the whole category.** Nearly every dedicated OSS drift tool has 0-4 stars. That may mean unmet demand, or that teams do not pay attention to this problem enough to adopt tooling. Demand evidence is anecdotal; I found no recent large survey or Reddit threads.
3. **False positives kill adoption.** Research shows heavy noise (DocPrism: 98% flagged naively; README paper: 0.29 user-facing accuracy best case). A noisy gate gets disabled within a week; ship warn-only first with baselines.
4. **The hard cases are semantic.** "The setup guide still works" can only be answered by executing it (Doc Detective's lane, with sandbox/security issues) or by LLM judgment (non-deterministic, cost, secrets handling for forks).
5. **LLM-vendor encroachment.** Mintlify, GitBook, Copilot and Claude-style skills are moving into "agent updates docs on PR". A detect-only OSS tool can be commoditised by a prompt, as doc-maintenance and doc-freshness skills already show.
6. **Ground-truth problem.** Which side is wrong, the doc or the code? Tools can only report inconsistency (DocTrail #5 says this explicitly). The README paper found ~40%+ of its "failures" were noisy labels, so benchmarking quality will be hard.
7. **Language/format sprawl.** Real value requires parsers per language (tree-sitter helps) and per doc format (MD, MDX, RST, AsciiDoc); scope creep is the usual killer.
8. **Naming and positioning.** "DriftGuard", "drift", "docs-drift", "driftcheck" are already taken or confusing across IaC/ML/docs.
9. **License contamination.** Borrowing from Commons Clause (borghei) or AGPL (Doc Detective) code is unsafe for a permissive project; clean-room required.
10. **CI cost/privacy.** Full git history is needed for git-based checks (Swimm notes shallow clones break it), and LLM stages need secrets that are unavailable on fork PRs (doc-drift-check handles by failing loudly).

---

## Executive summary

1. The brief's "451 stars" is wrong: the borghei repo has 891 stars but it is a skills monorepo; its doc-drift-detector is Python-only AST, git-timestamp heuristics, under MIT + Commons Clause (not OSI open source), with no packaged Action.
2. DriftGuard (lablab) is a LangGraph + Groq web demo with no CI story and no license; "DriftGuard" is also the name of 3+ unrelated repos.
3. The arkhe doc-freshness and paperclip doc-maintenance skills are agent prompts plus light scripts; readme-doc-drift-detector has no verifiable source repo.
4. Real OSS competitors exist that the brief did not list: pallaprolus/drift (CLI + Action, 5 languages, deterministic, MIT), docs-drift, doc-drift-check, DOCER (dormant). All have 0-4 stars, so the category is unclaimed, not proven.
5. Commercial vendors (Swimm, Mintlify, GitBook, DeepDocs, CodeRabbit) mostly sell LLM "generate the fix"; only Swimm does deterministic anchor-based PR gating, and it is proprietary.
6. Adjacent OSS (lychee, Vale, Doc Detective, oasdiff, byexample) each cover one slice; none verifies hand-written README/ADR/spec claims against code.
7. Research (arXiv 2603.00489, DocPrism, CASCADE) shows LLM-only detection is noisy: best README result gives about 1 valid recommendation per 4; DocPrism over-flags 98% without filtering. Deterministic-first with an optional LLM stage is the evidence-backed design.
8. Demand evidence is real but soft: 93% of OSS survey respondents see outdated docs (2017), over 25% of top-1000 projects had outdated references, and several repos independently file "fail CI on docs drift" issues; no Reddit signals found.
9. Best gap: a language-agnostic, deterministic, precision-first CLI/Action for claims in README/ADR/spec/onboarding docs (flags, env vars, routes, paths, versions, scripts) with doc-to-code mapping, baselines, stable exit codes and a permissive license.
10. Biggest risks: crowded young field, false-positive fatigue, ground-truth ambiguity, LLM-vendor commoditisation, and a lack of proven willingness to adopt or pay.
