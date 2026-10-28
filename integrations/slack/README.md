# Slack integration for ADK Go

Module `google.golang.org/adk/integrations/slack`. A separate Go module, like
`plugin/agentanalytics`, so the core `google.golang.org/adk/v2` module carries
no Slack code or dependencies.

Status: in progress. Packages land one pull request at a time:

- `mrkdwn` — converts the Markdown that models write into Slack mrkdwn. Available.
- the Slack service (Events API transport, answer policy, delivery), its `web`
  sublauncher, Socket Mode, and the Agent Runtime backend — upcoming.
