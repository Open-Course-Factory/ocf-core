#!/usr/bin/env python3
"""Create the public plan catalogue in an environment where the seed does not run.

`SetupDefaultSubscriptionPlans` is gated on ENVIRONMENT=development|test, so
production never seeds itself — deliberately, because creating commercial
artifacts is not something a pod restart should do. This script is how that
environment gets its catalogue, from the SAME definition the seed reads
(src/payment/services/catalogue.json), so the two cannot drift.

It is idempotent: a plan whose name already exists is reported and left alone,
never duplicated or overwritten. Run it as many times as you like.

    export OCF_API=https://api.example.org
    export OCF_ADMIN_TOKEN=...          # a platform administrator's JWT
    python3 scripts/bootstrap_catalogue.py --dry-run
    python3 scripts/bootstrap_catalogue.py

Stripe products and prices are NOT created here. Creating a plan fires the
async sync hook, but the authoritative step is the admin sync endpoint, which
this script only ever calls in dry-run mode — pushing prices to a live Stripe
account is a decision that deserves a human reading the plan of record first.
It prints the exact call to make.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

CATALOGUE = Path(__file__).resolve().parent.parent / "src" / "payment" / "services" / "catalogue.json"

# Fields the API refuses or ignores on create. `is_default_free` is elected at
# startup, never chosen per request; `is_catalog` needs the separate PATCH below.
NOT_ON_CREATE = {"is_default_free"}


def api(method, path, token, base, payload=None):
    url = base.rstrip("/") + path
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req) as resp:
            body = resp.read().decode()
            return resp.status, (json.loads(body) if body else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def existing_plans(token, base):
    status, body = api("GET", "/api/v1/subscription-plans", token, base)
    if status != 200:
        sys.exit(f"cannot list plans ({status}): {body}")
    rows = body if isinstance(body, list) else body.get("data", [])
    return {row["name"]: row for row in rows}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true", help="show what would be created, change nothing")
    ap.add_argument("--api", default=os.environ.get("OCF_API", "http://localhost:8080"))
    ap.add_argument("--token", default=os.environ.get("OCF_ADMIN_TOKEN", ""))
    args = ap.parse_args()

    if not args.token:
        sys.exit("set OCF_ADMIN_TOKEN to a platform administrator's JWT")

    catalogue = json.loads(CATALOGUE.read_text())
    present = existing_plans(args.token, args.api)

    print(f"catalogue: {len(catalogue)} plans defined, {len(present)} already in {args.api}\n")

    for plan in catalogue:
        name = plan["name"]
        if name in present:
            print(f"  = {name!r} already exists ({present[name]['id']}) — left alone")
            continue

        payload = {k: v for k, v in plan.items() if k not in NOT_ON_CREATE}
        hidden = not payload.get("is_catalog", True)

        if args.dry_run:
            price = payload["price_amount"] / 100
            print(f"  + would create {name!r} at {price:.2f} {payload['currency'].upper()}"
                  + (" (hidden)" if hidden else ""))
            continue

        status, body = api("POST", "/api/v1/subscription-plans", args.token, args.api, payload)
        if status not in (200, 201):
            print(f"  ! failed to create {name!r} ({status}): {body}")
            continue
        plan_id = body["id"]
        print(f"  + created {name!r} ({plan_id})")

        # is_catalog=false is dropped on create (#447): the plan lands visible on
        # the public pricing page whatever the payload said. Write it back, then
        # re-read — a hidden plan that is quietly public is the failure mode this
        # whole step exists to prevent.
        if hidden:
            status, body = api("PATCH", f"/api/v1/subscription-plans/{plan_id}",
                               args.token, args.api, {"is_catalog": False})
            if status not in (200, 204):
                print(f"  ! failed to hide {name!r} ({status}): {body}")
                continue
            status, body = api("GET", f"/api/v1/subscription-plans/{plan_id}", args.token, args.api)
            if status != 200 or body.get("is_catalog"):
                print(f"  ! {name!r} is STILL on the public catalogue — fix before announcing prices")
            else:
                print(f"    hidden from the public catalogue")

    print("\nStripe is not touched by this script. Next, in order:")
    print(f"  curl -X POST '{args.api}/api/v1/subscription-plans/sync-stripe?dry_run=true'  -H 'Authorization: Bearer …'")
    print("  # read the plan of record: expect `created` for each paid plan, no `archived`")
    print(f"  curl -X POST '{args.api}/api/v1/subscription-plans/sync-stripe?dry_run=false' -H 'Authorization: Bearer …'")
    print("\nThen confirm every created price carries the tax_behavior its plan declares")
    print("in catalogue.json — inclusive for the announced-TTC plans, exclusive for any")
    print("plan quoted net. EUR infers INCLUSIVE when unset, and the setting is one-way")
    print("per price, so a wrong one is only fixable by creating another price.")


if __name__ == "__main__":
    main()
