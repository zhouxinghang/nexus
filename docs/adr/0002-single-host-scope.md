# Single-host scope, no multi-machine agents

Nexus monitors and controls exactly one Host: the machine running the Nexus
process. Monitoring multiple machines would require deploying agents on each and
a network protocol between them — a fundamentally different architecture. We
explicitly exclude multi-host support from v1 so this boundary is understood as
a deliberate decision, not an oversight to be "fixed" later.
