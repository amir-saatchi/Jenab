# Jenab

A desktop AI agent that keeps its work in per-project databases, not long chats, and turns it into reusable dashboards and scheduled pipelines. Dashboards are built from tables, charts, stats and forms. Any model, local or cloud. Go + Wails.

> **Status:** in design. There is no app to use yet: the design, the spikes that tested it, the UI mockups and the first code skeleton are here. Phase 1 coding starts after Gate 3, with the [Phase 1 tickets](Docs/Tickets/README.md#phase-1).

## Why

Today's agent apps keep what they learn in the chat. Long chats waste tokens, get less accurate as they grow, and leave nothing lasting behind. Jenab gives every project its own folder and database:

- The agent stores what it finds in **tables**, and remembers intent in a short **project memory**.
- It builds **dashboards** from tables, charts, stats and forms, which stay up to date.
- **Pipelines** fetch and store data on a schedule, without a model in the loop unless a step needs one.
- Changes are logged, so a chat turn or a pipeline run can be undone.

It works with Anthropic, OpenAI, Gemini, OpenAI-compatible APIs and Ollama, with your own keys.

## Read more

- [Proposal](Docs/PROPOSAL.md): the idea, scope and roadmap
- [Spec](Docs/SPEC.md): storage, context, views, pipelines, concurrency and agent tools
- [Gates](Docs/Gates): vision, requirements, technical design, readiness and tests
- [Code design](Docs/Code-Design): how the Go code is structured
- [Development](Docs/DEVELOPMENT.md): build, run and test
- [Tickets](Docs/Tickets): the spikes and their results; their code is in [`spikes/`](spikes)
- [Mockups](mockups): the high-fidelity UI, built with React and shadcn/ui

## License

[Apache 2.0](LICENSE). Contributions are welcome under the [DCO](CONTRIBUTING.md).
