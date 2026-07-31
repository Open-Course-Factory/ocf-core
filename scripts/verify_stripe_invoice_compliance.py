#!/usr/bin/env python3
"""Check a Stripe account against the French invoice mentions obligatoires (#387).

Why a script and not a Go test: none of this is assertable from code. Invoices
are 100% Stripe-generated, and three of the settings can only be changed in the
Stripe Dashboard — `POST /v1/accounts/{own_account}` refuses with "You cannot use
this method on your own account". So the go-live gate is a check you run against
the configured account, twice: once in test, once in live.

The trap this exists to catch: `automatic_tax.status` read "complete" both while
Stripe Tax was computing 20% and while it was computing nothing at all. The
status field is not evidence. Only the tax amounts on a real invoice are.

READ-ONLY. It creates nothing, in either mode. The invoice section inspects the
most recent existing invoice, so running it against live is safe and does not
consume an invoice number.

Usage:
    scripts/verify_stripe_invoice_compliance.py              # configured (test) key
    scripts/verify_stripe_invoice_compliance.py --live       # live key

Requires the Stripe CLI, logged in (`stripe login`).
"""
import argparse
import json
import re
import subprocess
import sys

ANSI = re.compile(r"\x1b\[[0-9;]*m")

# Registry data for Labinux, SIREN 102 993 524. 293 B does not apply, so the VAT
# number must appear and a real VAT line must be computed.
EXPECTED_VAT = "FR82102993524"
EXPECTED_COUNTRY = "FR"
EXPECTED_CITY = "Toulouse"
FOOTER_MUST_MENTION = ["102 993 524", "TVA", "capital"]


def stripe_get(path, *params, live=False):
    """Call the Stripe CLI. Strips the claude-code plugin hint line and colours."""
    cmd = ["stripe", "get", path] + [a for p in params for a in ("-d", p)]
    if live:
        cmd.append("--live")
    r = subprocess.run(cmd, capture_output=True, text=True, stdin=subprocess.DEVNULL)
    raw = ANSI.sub("", (r.stdout or "") + (r.stderr or ""))
    start = min((i for i in (raw.find("{"), raw.find("[")) if i != -1), default=-1)
    if start == -1:
        return {"error": {"message": raw.strip()[:300] or "no output from stripe CLI"}}
    try:
        # raw_decode, not loads: the CLI (and the claude-code plugin) can emit
        # trailing lines after the JSON body, which loads() rejects outright.
        return json.JSONDecoder().raw_decode(raw[start:])[0]
    except json.JSONDecodeError:
        return {"error": {"message": raw[start:start + 300]}}


class Report:
    """Two sections, deliberately separate.

    Account configuration is current and authoritative. The invoice section is
    historical: it shows how Stripe rendered one particular invoice under the
    settings in force at the time. Reading them as one list is how a
    long-fixed gap gets re-reported, or a current one gets missed.
    """

    def __init__(self):
        self.sections = {}

    def check(self, section, label, ok, detail):
        self.sections.setdefault(section, []).append((label, ok, detail))

    def render(self):
        gaps = 0
        width = max(len(l) for rows in self.sections.values() for l, _, _ in rows)
        for name, rows in self.sections.items():
            print(f"{name}\n")
            for label, ok, detail in rows:
                print(f"  [{'PASS' if ok else 'GAP '}] {label.ljust(width)}  {detail}")
                if not ok:
                    gaps += 1
            print()
        if gaps:
            print(f"{gaps} gap(s). Not ready to invoice from this account.")
        else:
            print("All checks pass for this account.")
        return gaps


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--live", action="store_true", help="check the live account instead of test")
    args = ap.parse_args()
    live = args.live

    acct = stripe_get("/v1/account", live=live)
    if "error" in acct:
        sys.exit(f"cannot read account: {acct['error']['message']}")

    mode = "LIVE" if live else "TEST"
    print(f"\nStripe account {acct.get('id')} — {mode} mode\n")

    rep = Report()
    CFG = "ACCOUNT CONFIGURATION (current)"
    INV = "LAST INVOICE (historical — only meaningful if issued AFTER the settings above)"

    # --- Seller identity, as it prints on the PDF -------------------------
    bp = acct.get("business_profile") or {}
    addr = bp.get("support_address") or {}
    line1 = addr.get("line1") or ""
    placeholder = line1.startswith("address_") or not line1
    rep.check(
        CFG, "Seller address",
        not placeholder and addr.get("country") == EXPECTED_COUNTRY,
        f"{line1!r}, {addr.get('city')}, {addr.get('country')}"
        + ("  <- Stripe test placeholder, not a real address" if placeholder else ""),
    )
    rep.check(CFG, "Seller name", bool(bp.get("name")), repr(bp.get("name")))

    # --- Stripe Tax ------------------------------------------------------
    tax = stripe_get("/v1/tax/settings", live=live)
    status = tax.get("status")
    missing = ((tax.get("status_details") or {}).get("pending") or {}).get("missing_fields")
    rep.check(
        CFG, "Stripe Tax active", status == "active",
        f"status={status}" + (f", missing={missing}" if missing else ""),
    )
    rep.check(
        CFG, "Head office set", bool((tax.get("head_office") or {}).get("address")),
        json.dumps((tax.get("head_office") or {}).get("address"), ensure_ascii=False),
    )
    behavior = (tax.get("defaults") or {}).get("tax_behavior")
    rep.check(
        CFG, "Account default tax behaviour", behavior == "exclusive",
        f"{behavior} — 'inferred_by_currency' resolves EUR to INCLUSIVE, "
        "which silently turns announced prices into gross",
    )

    regs = stripe_get("/v1/tax/registrations", "status=all", live=live)
    fr_active = [r for r in regs.get("data", [])
                 if r.get("country") == EXPECTED_COUNTRY and r.get("status") == "active"]
    rep.check(
        CFG, "FR tax registration", bool(fr_active),
        f"{len(fr_active)} active FR registration(s) — without one Stripe Tax computes ZERO "
        "while still reporting automatic_tax=complete",
    )

    # --- Seller VAT number ----------------------------------------------
    tax_ids = stripe_get("/v1/tax_ids", live=live)
    values = [t.get("value") for t in tax_ids.get("data", [])]
    rep.check(CFG, "Account VAT tax ID exists", EXPECTED_VAT in values, f"{values}")

    defaults = ((acct.get("settings") or {}).get("invoices") or {}).get("default_account_tax_ids")
    rep.check(
        CFG, "VAT number default on invoices", bool(defaults),
        f"settings.invoices.default_account_tax_ids={defaults} "
        "(Dashboard > Billing > Invoices)",
    )

    # --- Prices ----------------------------------------------------------
    prices = stripe_get("/v1/prices", "limit=100", "active=true", live=live)
    unspecified = [p["id"] for p in prices.get("data", []) if p.get("tax_behavior") == "unspecified"]
    rep.check(
        CFG, "Every active price is tax-exclusive", not unspecified,
        f"{len(prices.get('data', []))} active price(s), {len(unspecified)} still 'unspecified'"
        + (f" {unspecified[:3]}" if unspecified else "")
        + " — tax_behavior can be set ONCE and never changed",
    )

    # --- A real invoice, which is the only actual evidence ---------------
    #
    # Finalized only. A draft has no number yet and has not had the account tax
    # IDs applied, so inspecting one reports gaps that do not exist.
    # Evidence has to be an invoice that actually charged something. A draft has
    # no number and no tax IDs applied yet; a zero-total invoice has no VAT to
    # compute, so it reports "no VAT" and looks like a failure. Both would make
    # this check lie. Among the rest, prefer one that still stands, since a void
    # invoice may predate the settings being checked.
    rank = {"paid": 0, "open": 1, "uncollectible": 2, "void": 3}
    invoices = stripe_get("/v1/invoices", "limit=20", live=live)
    finalized = sorted(
        (i for i in (invoices.get("data") or [])
         if i.get("status") != "draft" and (i.get("total") or 0) > 0),
        key=lambda i: (rank.get(i.get("status"), 9), -(i.get("created") or 0)),
    )
    if not finalized:
        rep.check(INV, "Invoice rendering", False,
                  "no finalized non-zero invoice on this account — issue one and re-run; "
                  "the footer and VAT line cannot be verified any other way")
    else:
        inv = finalized[0]
        num = inv.get("number")
        taxes = inv.get("total_taxes") or inv.get("total_tax_amounts") or []
        auto = (inv.get("automatic_tax") or {}).get("status")
        footer = inv.get("footer") or ""

        rep.check(INV, "Sequential numbering", bool(num), f"{num}")
        # A zero-amount entry is not computed VAT. Stripe emits one when Tax runs
        # with no registration to apply, which is precisely the state that looked
        # healthy for weeks: automatic_tax "complete", VAT nil.
        charged = sum(t.get("amount") or 0 for t in taxes)
        rep.check(
            INV, "VAT actually computed", charged > 0,
            f"automatic_tax={auto}, VAT charged={charged}"
            + ("  <- automatic_tax says complete while computing nothing"
               if auto == "complete" and charged == 0 else ""),
        )
        rep.check(
            INV, "VAT charged on top, not inside",
            all(t.get("tax_behavior") == "exclusive" for t in taxes) if taxes else False,
            f"behaviours={[t.get('tax_behavior') for t in taxes]}",
        )
        rep.check(
            INV, "Seller VAT number on the invoice", bool(inv.get("account_tax_ids")),
            f"account_tax_ids={inv.get('account_tax_ids')}",
        )
        missing_mentions = [m for m in FOOTER_MUST_MENTION if m.lower() not in footer.lower()]
        rep.check(
            INV, "Footer carries SIRET / capital / VAT", bool(footer) and not missing_mentions,
            (f"missing: {missing_mentions}" if footer else "footer empty")
            + " — Stripe has no structured field for these; the footer is the only place",
        )
    gaps = rep.render()
    if finalized:
        ev = finalized[0]
        stale = " — VOID and possibly predating the current settings; issue a fresh invoice" \
            if ev.get("status") == "void" else ""
        print(f"\nEvidence: invoice {ev.get('id')} ({ev.get('number')}, {ev.get('status')}){stale}")
    return gaps


if __name__ == "__main__":
    sys.exit(1 if main() else 0)
