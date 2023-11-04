# Index Supply, Co.

A note on maturity.

I am working towards a November 3rd, 2023 v1 release. Between now and then I will be making breaking changes to the database schema. After v1 there will be a protocol for introducing non-breaking changes. Until then things will break. A database reset will be required for breaking changes.

There is no auth required for RLPS. After November 3rd, RLPS will require auth and there will be pricing in place. There will be free, hobby, and pro tiers. I am hoping to make the pricing something like $50 and $500 per month for hobby and pro respectively.

**Nov 3rd update**

I should have known. I'm a bit behind on the Nov 3rd v1 release. I spent a couple of weeks tuning the backfill system and E2PG is much better for it. That being said, it's going to take another week or so to get to v1.

The goal for v1 is to provide a stable database schema so that future updates are easy. I think we are at that point now. I will spend next week testing, testing some more, and then I'll do a bit more testing.

[Vision][1]

[E2PG][2] Ethereum to Postgres Indexer

[RLPS][3] Low Level Blockchain Data API

[Run your own node][4]

[1]: https://github.com/orgs/indexsupply/discussions/125
[2]: https://github.com/orgs/indexsupply/discussions/122
[3]: https://github.com/orgs/indexsupply/discussions/123
[4]: https://github.com/orgs/indexsupply/discussions/124
