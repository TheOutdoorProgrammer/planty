package agent

// Trusted are the only hostnames the model may read from, and Sources is what
// it is told about them. Both live here so the permission rules and the prompt
// cannot disagree about what is allowed.
//
// Every one was checked with the same fetcher the service uses, because a site
// that refuses a plain GET is useless here however authoritative it is. That
// test is what removed Missouri Botanical Garden, whose plant finder redirects
// across hosts to somewhere the fetcher will not follow.
var Trusted = []string{
	"plants.ces.ncsu.edu",
	"www.aspca.org",
	"ipm.ucanr.edu",
	"ohioline.osu.edu",
	"extension.psu.edu",
}

// Sources is appended to the system prompt. Written as instructions about when
// to reach for each, because a bare list of domains tells a model nothing
// about which one answers the question in front of it.
const Sources = `You can search the web and read pages, but only from these five
sites. Nothing else is reachable, so do not try.

  plants.ces.ncsu.edu   NC State Extension's plant toolbox. Species-specific
                        care: light, water, soil, humidity. Each species page
                        also carries its own toxicity section. Start here for
                        "what does this plant actually want".

  www.aspca.org         The authority on whether a plant is poisonous to cats
                        or dogs. Use it whenever pet safety is the question,
                        and prefer it over any second-hand claim.

  ipm.ucanr.edu         UC's pest and disease notes: fungus gnats, mealybugs,
                        scale, spider mites and the rest, with identification
                        and what to do. Indoor pests are the same everywhere,
                        so its California origin does not matter here.

  ohioline.osu.edu      Ohio State Extension. The one to use for anything that
                        depends on being in Ohio: frost dates, planting and
                        harvest timing, growing edibles in this climate.

  extension.psu.edu     Penn State Extension. Best on mushroom growing, and
                        good on plant disease generally.

Rules about using them:

- The plant's own record beats anything on the web. Look things up to explain
  or to fill a genuine gap, never to replace what you were told.
- Say which site an answer came from when you used one.
- A search that finds nothing useful is a fine outcome. Say so rather than
  reaching for something half-remembered.`
