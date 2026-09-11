# Interview Notes

## One-Minute Pitch

I built Harness Onboarding Agent, an AI DevOps workbench that starts from a repository URL, infers project defaults, recommends a CI strategy, and generates Harness pipeline configuration for Go services. The frontend provides a visual pipeline editor using React Flow, while the Go backend turns validated project metadata into Harness-compatible YAML. The product is designed around Preview, Demo, and Connected modes so users can get value before sharing sensitive credentials.

## Why This Project Is Different

It combines frontend product thinking, agent-style automation, Go backend design, and DevOps delivery concepts. It is not just a CRUD app: the core value is turning project context into an executable delivery workflow.

## Technical Talking Points

- React state drives both form input and pipeline visualization.
- React Flow renders build stages as inspectable nodes.
- The analyze endpoint infers service name, Docker image name, and project context from repository metadata.
- Preview Mode lists the Harness account, token, connector, and secret setup required before a real run.
- Demo Mode is designed around a sandbox and whitelisted repositories instead of arbitrary user code.
- Connected Mode keeps Harness tokens on the backend and uses them only for Harness API calls.
- The Go backend owns pipeline generation so Harness integration stays server-side.
- The current Agent Analysis is deterministic in v1, which makes the demo stable.
- The architecture leaves room for an LLM planner, repository scanning, and real Harness API calls.

## Harness Explanation

Harness is like an automated release operator. A pipeline tells Harness what to do: clone code, run tests, build the service, package the image, and deploy it. This project focuses on generating and visualizing that pipeline instead of making developers write it by hand.

## Honest Agent Framing

The current version is not a fully autonomous LLM agent yet. It is a rule-based agent workflow: analyze inputs, recommend a strategy, generate a pipeline, and visualize the result. The production version would call an LLM for project understanding, then let Go validate the structured output and generate the final YAML.
