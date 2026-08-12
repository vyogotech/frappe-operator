# Licensing

Frappe Operator is licensed under the **Elastic License 2.0 (ELv2)**. The full
legal text is in [LICENSE](LICENSE). This document is a plain-English summary —
where this summary and the license text differ, **the [LICENSE](LICENSE) text
controls**.

## What you CAN do for free

The Elastic License 2.0 is a **source-available** license. You may, at no cost:

- **Use** Frappe Operator to run, deploy, and manage Frappe/ERPNext on your own
  Kubernetes clusters — including in production, for your own business or your
  own organization's internal users.
- **Self-host** it for yourself and for your own end customers' dedicated
  instances that you operate on their behalf.
- **Copy, modify, and redistribute** the source, and prepare derivative works.
- **Evaluate, develop, and test** without restriction.

You do **not** need to buy a license for any of the above.

## What requires a commercial license

The one core restriction of the Elastic License 2.0 is the **hosted / managed
service** limitation. Specifically:

> You may **not provide Frappe Operator to third parties as a hosted or managed
> service** where that service gives users access to a substantial set of the
> operator's features or functionality.

In practical terms, you need a **commercial license from Vyogo** if you want to:

- Offer a **Frappe/ERPNext cloud** or **Platform-as-a-Service** to third parties
  where Frappe Operator provides the provisioning/orchestration substrate.
- Run a **managed hosting service** or **multi-tenant SaaS** for external
  customers built on top of Frappe Operator's functionality (site provisioning,
  bench management, autoscaling, backups, etc.).
- Embed Frappe Operator into a commercial product you offer to third parties as
  a managed service.

If that describes your use case — for example, you are building a "Frappe Cloud"
competitor or a managed ERPNext hosting business — please contact us **before**
deploying in production:

📬 **dev@vyogo.tech** · 🌐 [vyogo.tech](https://vyogo.tech)

The other two ELv2 limitations also apply: you may not circumvent any license
key functionality, and you may not remove or obscure the licensing/copyright
notices.

## Relationship to Frappe and ERPNext

Frappe Operator is an **independent orchestration tool**. It does not include,
link against, or distribute Frappe or ERPNext source code — it deploys and runs
them as separate processes/containers via the `bench` CLI and the Kubernetes
API. As a result, the operator's Elastic License 2.0 is independent of the
licenses of the software it manages:

- **Frappe Framework** is MIT-licensed (permissive) — fully compatible.
- **ERPNext** is GPLv3-licensed. Frappe Operator interacts with it only at
  arm's length (separate processes/containers), so no combined/derivative work
  is formed. Any container images that bundle ERPNext remain governed by
  **GPLv3 as separate deliverables** (mere aggregation); this license applies
  only to the Frappe Operator code itself.

"Frappe" and "ERPNext" are trademarks of Frappe Technologies Pvt. Ltd. Use of
those marks is subject to their trademark policies and is not granted by this
license.

## Version history

- **v4.1.x and earlier** (every release before v4.2.0) was published under the Apache License 2.0. Those releases
  remain available under Apache 2.0 — this license change applies **going
  forward**.
- **From this release onward**, Frappe Operator is licensed under the Elastic
  License 2.0 as described above.

## Contributions

Contributions to Frappe Operator are accepted under the Elastic License 2.0.
Contributors may be asked to sign a Contributor License Agreement (CLA) so that
contributions can be distributed under these terms.

## Questions

Not sure whether your use needs a commercial license? Reach out — we're happy to
clarify: **dev@vyogo.tech**.
