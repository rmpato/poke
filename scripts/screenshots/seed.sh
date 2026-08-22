#!/bin/sh
# Produce the history the documentation screenshots are taken from.
#
#   scripts/screenshots/seed.sh ./bin/pogo /tmp/pogo-shots
#
# Every request is real: curl runs, pogo records it, and the demo server on
# 127.0.0.1:8080 answers. The hostnames are real too — curl's --resolve points
# them at the local server — because the whole point of the pictures is to show
# what pogo does with a spread of hosts, and "localhost:8080" five times over
# would show nothing.
#
# The requests are deliberately spread across one API in three environments and
# two other APIs, which is the shape the list is built to make sense of.
set -eu

POGO="${1:-./bin/pogo}"
export POGO_HOME="${2:-/tmp/pogo-shots}"
export POGO_CONFIG="$POGO_HOME/config.yaml"
export POGO_ENV_FILE="$POGO_HOME/environments.yaml"
SERVER="${SERVER:-127.0.0.1:8080}"

# Ages are what make the list look like a working day rather than a fixture, so
# requests are spaced out. Override for a quick iteration.
GAP="${GAP:-40}"

rm -rf "$POGO_HOME"
mkdir -p "$POGO_HOME"

say() { printf '\033[36m==>\033[0m %s\n' "$*"; }

# request <host> <method-and-path...>
#
# The response body is *not* discarded with -o: pogo captures what curl writes
# to stdout, so a request run with -o /dev/null is recorded with no body, and
# every body pane in the screenshots comes out empty. Redirect at the shell
# instead — curl still writes, pogo still tees, and the terminal stays quiet.
request() {
  host=$1
  shift
  "$POGO" curl -s \
    --resolve "$host:8080:${SERVER%%:*}" \
    "$@" >/dev/null || true
  sleep "$GAP"
}

AUTH="Authorization: Bearer sk-live-4f9c2a8e1b7d"

# Oldest first: groups are ordered by their most recent request, so the API you
# were working on last is the one at the top of the list — which is what makes
# the ordering worth having.
say "the one running on this machine"
"$POGO" curl -s "http://localhost:8080/users" >/dev/null || true
sleep "$GAP"

say "somebody else's API"
request api.stripe.com -H "$AUTH" http://api.stripe.com:8080/v1/charges
request api.stripe.com -H "$AUTH" http://api.stripe.com:8080/v1/customers

say "development"
request dev-api.acme.com -X DELETE http://dev-api.acme.com:8080/users/41
request dev-api.acme.com http://dev-api.acme.com:8080/nowhere

say "production"
request api.acme.com http://api.acme.com:8080/users
request api.acme.com -H "$AUTH" http://api.acme.com:8080/billing/invoices
request api.acme.com -H "$AUTH" http://api.acme.com:8080/users/42

say "staging"
request api.staging.acme.com -H "$AUTH" http://api.staging.acme.com:8080/users
request api.staging.acme.com -X POST -H 'Content-Type: application/json' \
  -d '{"name":"Ada"}' http://api.staging.acme.com:8080/users
request api.staging.acme.com http://api.staging.acme.com:8080/orders/9021
request api.staging.acme.com http://api.staging.acme.com:8080/orders/9021

say "history is in $POGO_HOME"
"$POGO" api list
