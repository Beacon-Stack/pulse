package indexer

// builtinCatalog contains the built-in indexer definitions.
// Modeled after Prowlarr's Cardigann YAML catalog structure.
// Each entry represents a template that can be configured and added.
var builtinCatalog = []CatalogEntry{
	// ── Public Torrent ───────────────────────────────────────────────────
	{ID: "1337x", Name: "1337x", Description: "1337x is a public torrent site offering verified torrents for movies, TV shows, games, music, and software.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://1337x.to", "https://1337x.st"}, Settings: []Field{
		{Name: "sort", Type: "select", Label: "Sort requested from site", Default: "time", Options: []FieldOption{{Name: "created", Value: "time"}, {Name: "seeders", Value: "seeders"}, {Name: "size", Value: "size"}}},
	}},
	{ID: "thepiratebay", Name: "The Pirate Bay", Description: "The Pirate Bay is the galaxy's most resilient public BitTorrent site.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://thepiratebay.org"}, Settings: []Field{}},
	{ID: "yts", Name: "YTS", Description: "YTS.MX is a public torrent site specializing in small-size, high-quality movie releases (720p/1080p/2160p).", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies"}, URLs: []string{"https://yts.mx"}, Settings: []Field{}},
	{ID: "eztv", Name: "EZTV", Description: "EZTV is a public torrent site focused on TV show releases with a large catalog of current and older series.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"TV"}, URLs: []string{"https://eztv.re"}, Settings: []Field{}},
	{ID: "limetorrents", Name: "LimeTorrents", Description: "LimeTorrents is a public torrent search engine indexing verified torrents across all categories.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://www.limetorrents.lol"}, Settings: []Field{}},
	{ID: "torrentgalaxy", Name: "TorrentGalaxy", Description: "TorrentGalaxy is a public torrent site with a community-driven catalog featuring IMDB ratings and screenshots.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other", "XXX"}, URLs: []string{"https://torrentgalaxy.to"}, Settings: []Field{}},
	{ID: "kickasstorrents", Name: "KickassTorrents", Description: "KickassTorrents is a public torrent search engine with a large and varied catalog.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://kickasstorrents.to"}, Settings: []Field{}},
	{ID: "nyaa", Name: "Nyaa", Description: "Nyaa is a public BitTorrent tracker for anime, manga, and East Asian media.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"TV", "Audio", "Books", "Other"}, URLs: []string{"https://nyaa.si"}, Settings: []Field{
		{Name: "cat_filter", Type: "select", Label: "Category filter", Default: "0_0", Options: []FieldOption{{Name: "All", Value: "0_0"}, {Name: "Anime", Value: "1_0"}, {Name: "Audio", Value: "2_0"}, {Name: "Literature", Value: "3_0"}}},
	}},
	{ID: "rutracker", Name: "RuTracker", Description: "RuTracker is one of the largest public Russian torrent trackers with an enormous catalog across all categories.", Language: "ru-RU", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://rutracker.org"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", HelpText: "Paste your session cookie value", Required: false, Placeholder: "bb_session=..."},
	}},
	{ID: "glodls", Name: "GloDLS", Description: "GloDLS is a public torrent tracker covering movies, TV shows, games, anime, and software.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://glodls.to"}, Settings: []Field{}},
	{ID: "torlock", Name: "Torlock", Description: "Torlock is a public torrent index that only lists verified torrents.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://www.torlock.com"}, Settings: []Field{}},
	{ID: "bitsearch", Name: "BitSearch", Description: "BitSearch is a clean public torrent search engine aggregating results from multiple sources.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://bitsearch.to"}, Settings: []Field{}},
	{ID: "academictorrents", Name: "Academic Torrents", Description: "Academic Torrents is a public tracker for sharing enormous academic datasets and research papers.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Books", "Other"}, URLs: []string{"https://academictorrents.com"}, Settings: []Field{}},
	{ID: "btdig", Name: "BTDigg", Description: "BTDigg is a public BitTorrent DHT search engine providing full-text search over the distributed hash table.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://btdig.com"}, Settings: []Field{}},

	// ── Semi-Private Torrent ─────────────────────────────────────────────
	{ID: "iptorrents", Name: "IPTorrents", Description: "IPTorrents is a large semi-private tracker with a vast catalog of movies, TV, music, games, and software.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://iptorrents.com"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", HelpText: "Login to the site and copy your cookie value.", Required: true, Placeholder: "uid=...; pass=..."},
	}},
	{ID: "torrentleech", Name: "TorrentLeech", Description: "TorrentLeech is a semi-private tracker known for fast pre-times and a large catalog.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://www.torrentleech.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "filelist", Name: "FileList", Description: "FileList is a Romanian semi-private tracker with a strong focus on HD/UHD movie and TV releases.", Language: "ro-RO", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://filelist.io"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "passkey", Type: "password", Label: "Passkey", HelpText: "Found in your profile settings.", Required: true},
	}},
	{ID: "drunkenslug", Name: "DrunkenSlug", Description: "DrunkenSlug is a semi-private usenet indexer with a generous free tier and broad category coverage.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "XXX"}, URLs: []string{"https://drunkenslug.com"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true, Placeholder: "Your API key from profile"},
	}},
	{ID: "scenetime", Name: "SceneTime", Description: "SceneTime is a semi-private tracker for movies, TV, music, and games with an active community.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://www.scenetime.com"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", Required: true},
	}},
	{ID: "torrentday", Name: "TorrentDay", Description: "TorrentDay is a semi-private tracker with a massive catalog and active community.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://www.torrentday.com"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", Required: true},
	}},
	{ID: "speedcd", Name: "Speed.cd", Description: "Speed.cd is a semi-private tracker known for fast speeds and well-organized releases.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://speed.cd"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", Required: true},
	}},
	{ID: "aither", Name: "Aither", Description: "Aither is a semi-private UNIT3D tracker focused on movies and TV shows.", Language: "en-US", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://aither.cc"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true, Placeholder: "From your profile → Security"},
	}},

	// ── Private Torrent ──────────────────────────────────────────────────
	{ID: "passthepopcorn", Name: "PassThePopcorn", Description: "PassThePopcorn (PTP) is an elite private tracker widely regarded as the best source for movie torrents.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies"}, URLs: []string{"https://passthepopcorn.me"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
		{Name: "passkey", Type: "password", Label: "Passkey", HelpText: "Found under Security in your profile.", Required: true},
	}},
	{ID: "broadcasthenet", Name: "BroadcasTheNet", Description: "BroadcasTheNet (BTN) is an elite private tracker for TV series with a comprehensive library.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"TV"}, URLs: []string{"https://broadcasthe.net"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "hdbits", Name: "HDBits", Description: "HDBits is an elite private tracker focused on high-quality HD and UHD movie and TV encodes.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://hdbits.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "passkey", Type: "password", Label: "Passkey", Required: true},
	}},
	{ID: "beyondhd", Name: "BeyondHD", Description: "BeyondHD (BHD) is a private tracker specializing in high-quality movie and TV encodes with a focus on remuxes.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://beyond-hd.me"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true, Placeholder: "From Security settings in profile"},
	}},
	{ID: "blutopia", Name: "Blutopia", Description: "Blutopia (BLU) is a private UNIT3D tracker for HD/UHD movies and TV shows.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://blutopia.cc"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "anthelion", Name: "Anthelion", Description: "Anthelion is a private Gazelle-based movie tracker with a growing library.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies"}, URLs: []string{"https://anthelion.me"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "torrentseeds", Name: "TorrentSeeds", Description: "TorrentSeeds is a private tracker with a wide variety of content including movies, TV, and games.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://torrentseeds.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "orpheus", Name: "Orpheus", Description: "Orpheus is a private music tracker (Gazelle-based) for FLAC, MP3, and other audio formats.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Audio"}, URLs: []string{"https://orpheus.network"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "redacted", Name: "Redacted", Description: "Redacted (RED) is an elite private music tracker — the successor to What.CD.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Audio"}, URLs: []string{"https://redacted.sh"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true, Placeholder: "Generate under Settings → Access"},
	}},
	{ID: "myanonamouse", Name: "MyAnonamouse", Description: "MyAnonamouse (MAM) is a private tracker for ebooks, audiobooks, and educational materials.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Books", "Audio"}, URLs: []string{"https://www.myanonamouse.net"}, Settings: []Field{
		{Name: "mam_id", Type: "text", Label: "MAM ID", HelpText: "Your session cookie value from mam_id cookie.", Required: true},
	}},
	{ID: "bibliotik", Name: "Bibliotik", Description: "Bibliotik is an elite private tracker for ebooks, audiobooks, and comics.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Books"}, URLs: []string{"https://bibliotik.me"}, Settings: []Field{
		{Name: "cookie", Type: "text", Label: "Cookie", Required: true},
	}},
	{ID: "gazellegames", Name: "GazelleGames", Description: "GazelleGames (GGn) is a private tracker for PC, console, and retro video games.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Other"}, URLs: []string{"https://gazellegames.net"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "empornium", Name: "Empornium", Description: "Empornium is the largest private XXX tracker with an extensive library of adult content.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"XXX"}, URLs: []string{"https://www.empornium.is"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "pornbay", Name: "PornBay", Description: "PornBay is a private tracker for adult content with a large and well-organized catalog.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"XXX"}, URLs: []string{"https://pornbay.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "animebytes", Name: "AnimeBytes", Description: "AnimeBytes (AB) is an elite private tracker for anime, manga, light novels, and Asian music.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"TV", "Audio", "Books"}, URLs: []string{"https://animebytes.tv"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "passkey", Type: "password", Label: "Passkey", Required: true},
	}},
	{ID: "cinemaz", Name: "CinemaZ", Description: "CinemaZ is a private tracker for rare and obscure films, foreign cinema, and documentaries.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies"}, URLs: []string{"https://cinemaz.to"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "avistaz", Name: "AvistaZ", Description: "AvistaZ is a private tracker for Asian movies, TV shows, and music.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://avistaz.to"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
		{Name: "pid", Type: "text", Label: "PID", HelpText: "Your passkey from profile.", Required: true},
	}},
	{ID: "privatehd", Name: "PrivateHD", Description: "PrivateHD is a private tracker for HD/UHD movies and TV shows, sister site to AvistaZ.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://privatehd.to"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
		{Name: "pid", Type: "text", Label: "PID", Required: true},
	}},
	{ID: "nebulance", Name: "Nebulance", Description: "Nebulance (NBL) is a private tracker focused exclusively on TV series.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"TV"}, URLs: []string{"https://nebulance.io"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "alpharatio", Name: "AlphaRatio", Description: "AlphaRatio (AR) is a private tracker with a good mix of movies, TV, games, and software.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Other"}, URLs: []string{"https://alpharatio.cc"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "ncore", Name: "nCore", Description: "nCore is the largest Hungarian private tracker with a huge catalog and active community.", Language: "hu-HU", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://ncore.pro"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
		{Name: "twofa", Type: "text", Label: "2FA Code", HelpText: "Only if two-factor auth is enabled.", Required: false},
	}},
	{ID: "morethantv", Name: "MoreThanTV", Description: "MoreThanTV (MTV) is a private Gazelle-based tracker for TV shows and movies.", Language: "en-US", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://www.morethantv.me"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},

	// ── Usenet Indexers ──────────────────────────────────────────────────
	{ID: "nzbgeek", Name: "NZBgeek", Description: "NZBgeek is a popular Newznab-based usenet indexer with a large catalog and active community.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "XXX"}, URLs: []string{"https://nzbgeek.info"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true, Placeholder: "Your NZBgeek API key"},
	}},
	{ID: "nzbfinder", Name: "NZBFinder", Description: "NZBFinder is a Newznab-based usenet indexer with both free and VIP tiers.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "XXX"}, URLs: []string{"https://nzbfinder.ws"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "nzbplanet", Name: "NZBPlanet", Description: "NZBPlanet is a usenet indexer with solid coverage of movies, TV, and music.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://nzbplanet.net"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "abnzb", Name: "abNZB", Description: "abNZB is a Newznab-based usenet indexer with a focus on completeness and retention.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books"}, URLs: []string{"https://abnzb.com"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "nzbsu", Name: "NZB.su", Description: "NZB.su is one of the first Newznab-based usenet indexers with a long history.", Language: "en-US", Protocol: "usenet", Privacy: "private", Categories: []string{"Movies", "TV", "Audio", "Books"}, URLs: []string{"https://nzb.su"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "nzbcat", Name: "NZBCat", Description: "NZBCat is a Newznab-based usenet indexer with good coverage and an active community.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://nzb.cat"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "dog", Name: "DOGnzb", Description: "DOGnzb is a premium usenet indexer known for quality and reliability.", Language: "en-US", Protocol: "usenet", Privacy: "private", Categories: []string{"Movies", "TV", "Audio", "Books"}, URLs: []string{"https://dognzb.cr"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "oznzb", Name: "OZnzb", Description: "OZnzb is an Australian usenet indexer with a focus on local and international content.", Language: "en-AU", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://oznzb.com"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "tabula-rasa", Name: "Tabula Rasa", Description: "Tabula Rasa is a Newznab-based usenet indexer with a clean interface and good automation support.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books"}, URLs: []string{"https://www.tabula-rasa.pw"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "althub", Name: "altHUB", Description: "altHUB is a usenet indexer with a focus on alt binaries and a Newznab API.", Language: "en-US", Protocol: "usenet", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://althub.co.za"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},

	// ── Generic / Custom ─────────────────────────────────────────────────
	{ID: "generic-torznab", Name: "Generic Torznab", Description: "Connect to any Torznab-compatible indexer (Jackett, Prowlarr proxy, custom). Provide the full API URL and key.", Language: "en-US", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Books", "XXX", "Other"}, URLs: []string{}, Settings: []Field{
		{Name: "url", Type: "text", Label: "Torznab URL", Required: true, Placeholder: "http://jackett:9117/api/v2.0/indexers/all/results/torznab"},
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
		{Name: "categories", Type: "text", Label: "Categories", HelpText: "Comma-separated Newznab category IDs (e.g., 2000,5000).", Required: false, Placeholder: "2000,5000"},
	}},
	{ID: "generic-newznab", Name: "Generic Newznab", Description: "Connect to any Newznab-compatible usenet indexer. Provide the full API URL and key.", Language: "en-US", Protocol: "usenet", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Books", "XXX", "Other"}, URLs: []string{}, Settings: []Field{
		{Name: "url", Type: "text", Label: "Newznab URL", Required: true, Placeholder: "https://indexer.example.com/api"},
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
		{Name: "categories", Type: "text", Label: "Categories", HelpText: "Comma-separated Newznab category IDs.", Required: false, Placeholder: "2000,5000"},
	}},

	// ── Non-English / Regional ───────────────────────────────────────────
	{ID: "t411", Name: "T411", Description: "T411 is a large French-language general torrent tracker with strong scene coverage.", Language: "fr-FR", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://t411.li"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "yggtorrent", Name: "YGGTorrent", Description: "YGGTorrent is the largest French-language general torrent tracker.", Language: "fr-FR", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://www.yggtorrent.qa"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "sharewood", Name: "Sharewood", Description: "Sharewood is a French semi-private tracker for movies, TV, music, and software.", Language: "fr-FR", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://www.sharewood.tv"}, Settings: []Field{
		{Name: "passkey", Type: "text", Label: "Passkey", Required: true},
	}},
	{ID: "cpasbien", Name: "Cpasbien", Description: "Cpasbien is a popular French public torrent site for movies and TV.", Language: "fr-FR", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://www.cpasbien.tw"}, Settings: []Field{}},
	{ID: "mejortorrent", Name: "MejorTorrent", Description: "MejorTorrent is the largest Spanish-language public torrent site.", Language: "es-ES", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV"}, URLs: []string{"https://mejortorrent.wtf"}, Settings: []Field{}},
	{ID: "dontorrent", Name: "DonTorrent", Description: "DonTorrent is a Spanish public torrent site for movies, series, and music.", Language: "es-ES", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://dontorrent.earth"}, Settings: []Field{}},
	{ID: "divxtotal", Name: "DivxTotal", Description: "DivxTotal is a Spanish public torrent site with a large catalog of dubbed content.", Language: "es-ES", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV"}, URLs: []string{"https://www.divxtotal.mov"}, Settings: []Field{}},
	{ID: "ilcorsaronero", Name: "ilCorSaRoNeRo", Description: "ilCorSaRoNeRo is the largest Italian public torrent tracker.", Language: "it-IT", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://ilcorsaronero.link"}, Settings: []Field{}},
	{ID: "tntvillage", Name: "TNTVillage", Description: "TNTVillage is an Italian semi-private tracker with a community-moderated library.", Language: "it-IT", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books"}, URLs: []string{"https://tntvillage.scambioetico.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "kinozal", Name: "Kinozal", Description: "Kinozal is a major Russian semi-private tracker for movies, TV, and music.", Language: "ru-RU", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://kinozal.tv"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "nnmclub", Name: "NNM-Club", Description: "NNM-Club is a large Russian torrent tracker covering movies, TV, software, and games.", Language: "ru-RU", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Other"}, URLs: []string{"https://nnmclub.to"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "toloka", Name: "Toloka", Description: "Toloka is a Ukrainian torrent tracker with a large community and diverse catalog.", Language: "uk-UA", Protocol: "torrent", Privacy: "semi-private", Categories: []string{"Movies", "TV", "Audio", "Books", "Other"}, URLs: []string{"https://toloka.to"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "brasiltrackers", Name: "BrasilTrackers", Description: "BrasilTrackers is the largest Brazilian Portuguese private tracker.", Language: "pt-BR", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://brasiltrackers.org"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "torrentland", Name: "TorrentLand", Description: "TorrentLand is a Spanish private tracker with a focus on HD movie and TV releases.", Language: "es-ES", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://torrentland.li"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "tokyotosho", Name: "Tokyo Toshokan", Description: "Tokyo Toshokan is a Japanese BitTorrent library for anime, manga, and Japanese media.", Language: "ja-JP", Protocol: "torrent", Privacy: "public", Categories: []string{"TV", "Audio", "Books", "Other"}, URLs: []string{"https://www.tokyotosho.info"}, Settings: []Field{}},
	{ID: "anidex", Name: "AniDex", Description: "AniDex is a public torrent tracker for anime, manga, and East Asian media.", Language: "ja-JP", Protocol: "torrent", Privacy: "public", Categories: []string{"TV", "Audio", "Books"}, URLs: []string{"https://anidex.info"}, Settings: []Field{}},
	{ID: "elitetorrent", Name: "EliteTorrent", Description: "EliteTorrent is a Spanish public torrent site for movies and series.", Language: "es-ES", Protocol: "torrent", Privacy: "public", Categories: []string{"Movies", "TV"}, URLs: []string{"https://elitetorrent.wf"}, Settings: []Field{}},
	{ID: "unionfansub", Name: "Union Fansub", Description: "Union Fansub is a Spanish anime fansubbing community with its own torrent tracker.", Language: "es-ES", Protocol: "torrent", Privacy: "public", Categories: []string{"TV"}, URLs: []string{"https://foro.unionfansub.com"}, Settings: []Field{}},
	{ID: "german-palast", Name: "German Palast", Description: "German Palast is a German private tracker for movies, TV, and music.", Language: "de-DE", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://german-palast.to"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "hdarea", Name: "HDArea", Description: "HDArea is a Chinese private tracker for HD movies and TV shows.", Language: "zh-CN", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://www.hdarea.co"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "chdbits", Name: "CHDBits", Description: "CHDBits is a prestigious Chinese private HD tracker.", Language: "zh-CN", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV"}, URLs: []string{"https://chdbits.co"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "passkey", Type: "password", Label: "Passkey", Required: true},
	}},
	{ID: "polishtracker", Name: "PolishTracker", Description: "PolishTracker is the largest Polish private tracker with Polish-dubbed and subtitled content.", Language: "pl-PL", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://pte.nu"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
	{ID: "danishbytes", Name: "DanishBytes", Description: "DanishBytes is a Danish private tracker for Danish-language and international content.", Language: "da-DK", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://danishbytes.club"}, Settings: []Field{
		{Name: "api_key", Type: "text", Label: "API Key", Required: true},
	}},
	{ID: "nordicbits", Name: "NordicBits", Description: "NordicBits is a Nordic private tracker for Scandinavian-language movies, TV, and music.", Language: "sv-SE", Protocol: "torrent", Privacy: "private", Categories: []string{"Movies", "TV", "Audio"}, URLs: []string{"https://nordicbits.net"}, Settings: []Field{
		{Name: "username", Type: "text", Label: "Username", Required: true},
		{Name: "password", Type: "password", Label: "Password", Required: true},
	}},
}
