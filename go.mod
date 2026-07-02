module github.com/Herrscherd/herrscher-llm-extractor

go 1.25

require (
	github.com/Herrscherd/herrscher-contracts v0.1.9
	github.com/Herrscherd/herrscher-orchestrator v0.1.4
)

// dev-only — dropped before release
replace github.com/Herrscherd/herrscher-contracts => /home/shan/dev/herrscher-contracts

replace github.com/Herrscherd/herrscher-orchestrator => /home/shan/dev/herrscher-orchestrator
