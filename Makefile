.PHONY: setup check lint test gotest verify docs repair-verify rollback-verify

setup:
	node scripts/dev.mjs setup

check:
	node scripts/dev.mjs check

lint:
	node scripts/dev.mjs check

test:
	node scripts/dev.mjs verify

gotest:
	go test ./...

verify:
	node scripts/dev.mjs verify

docs:
	node scripts/dev.mjs docs

repair-verify:
	node scripts/dev.mjs repair:verify

rollback-verify:
	node scripts/dev.mjs rollback:verify
