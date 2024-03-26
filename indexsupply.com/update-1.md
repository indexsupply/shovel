<title> Index Supply / Update 1</title>

# [Index Supply](/) / Update 1

_March 26th, 2024_

It's been nearly a year since I wrote the [announcement post](https://github.com/orgs/indexsupply/discussions/125) for Index Supply. Some of those ideas remain and some have changed. What hasn't changed is my personal excitement for the space. I got into crypto in 2012 when I was dealing with script kiddies who tried hijacking the Heroku free tier for Bitcoin mining. Since then I've been annoyed but also positively infected with the ideas of crypto currency. My excitement for Ethereum (and Bitcoin) increases year over year.

Index Supply started with the vision of building an open source [Ethereum client for indexing](https://github.com/orgs/indexsupply/discussions/129). I am still working towards that goal; albiet through a slight detour. I started by building an [Ethereum stack from scratch in Go](https://github.com/indexsupply/code/tree/main). I implemented RLP, ABI, and the Disc protocol --all with [improved performance](https://github.com/orgs/indexsupply/discussions/88) when compared with current tools. But along the way I was introduced to various teams who wanted me to build an Ethereum to Postgres indexer. And so I built [Shovel](/shovel). Shovel is an open source tool that uses a declaritive json config to map Ethereum events onto Postgres tables. It doesn't require custom code and it works concurrently on many different chains (ie maninet + base + zora etc.). Shovel is now in production on [many key projects](/customer-case-studies) and I recently announced its [1.0 release](/shovel/1.0).

While building Shovel, I have come across several unique projects that require highly tuned indexing solutions. I could try and make Shovel fit those use cases but if I were in charge of these unique projects, I would not want a generalized solution; instead I would want something tailored to the problem. And for these projects, Index Supply is spinning up an Engineering Services business.

I have a decade of experience building infrastructure (load balancers, linux servers, databases) and crypto services (bitcoin apis, private blockchains, ethereum nodes, indexers) and by combining these skills I can help teams fast forward to the point where they have a solid foundation from which they can hire and build upon.

We will use the tools that are best suited for the problem and invent new tools when necessary. We have our existing Go codebase to draw from but we are also building with Rust, Reth, Alloy, and REVM.

Index Supply remains funded by grants and revenue. We remain independent and free to explore unusual growth opportunities --such as Engineering Services. Our revenues have been growing steadily and the company is now in a good place to hire engineer #1. I am eager to find the right person.

The future is bright and I will share more updates on the projects that are already underway.
