import type { Catalog } from "../types";

export const settings: Catalog["settings"] = {
  settings: {
    page: {
      title: "Settings",
      intro: "Changes here take effect immediately, with no restart required. Durations accept {m5}, {s90}, {h168}. Credentials are never displayed again — to change one, enter the new value; to disable, restore the default.",
    },
    trackers: {
      title: "Trackers",
      blurb: "Sites where content is published.",
    },
    categories: {
      admission: "How the cluster splits the simultaneous downloads allowed by the tracker.",
      agent: "How Seed Nodes behave: Agent and Engine queue. This holds the CLUSTER value — it applies to every Node without its own value on the next heartbeat; a Node that handles more or less than the fleet gets its own setting in the Node detail. Also: where Nodes fetch their updates.",
      alerting: "When the Control Plane raises an alert (Node offline, disk, storage).",
      notify: "Routes alerts and events outside the Portal.",
      integrations: "Third-party service keys. They live here, never in code — they're yours.",
      workers: "Pace of the Control Plane's internal loops.",
      publish: "Content publishing to trackers.",
    },
    subsections: {
      notify: {
        telegram: {
          title: "Telegram",
          blurb: "@BotFather bot + destination. Active when token and chat_id are filled in.",
        },
        discord: {
          title: "Discord",
          blurb: "Webhook for a Discord channel.",
        },
        webhook: {
          title: "Webhook",
          blurb: "Raw JSON POST to your own URL — Slack, n8n, Home Assistant, you name it.",
        },
        rules: {
          title: "Delivery rules",
          blurb: "What gets sent through the channels above, plus the anti-flood limit.",
        },
      },
      integrations: {
        tmdb: {
          title: "TMDB",
          blurb: "Movie and series metadata for publishing. This is your key (themoviedb.org).",
        },
        igdb: {
          title: "IGDB",
          blurb: "Game metadata via a Twitch app.",
        },
        imgbb: {
          title: "imgbb",
          blurb: "Image host for publish screenshots.",
        },
        googleBooks: {
          title: "Google Books",
          blurb: "Secondary book/audiobook cover provider, for titles not indexed by Open Library.",
        },
      },
      other: {
        title: "Other",
        blurb: "Tunables in this category without their own card.",
      },
    },
    field: {
      clusterBadge: "cluster · per Node",
      clusterBadgeTitle: "This is the cluster-wide value. Each Node can have its own value, set in the Node detail.",
      modified: "modified",
      nothingSelected: "none selected",
      selectedCount: {
        one: "{count} selected",
        other: "{count} selected",
      },
      optionOn: "On",
      optionOff: "Off",
      placeholderSecretChange: "•••••••• — type to change",
      placeholderNotConfigured: "not configured",
      placeholderDuration: "5m, 90s, 168h",
      emptyValue: "empty",
    },
    status: {
      on: "on",
      off: "off",
    },
    actions: {
      clear: "Clear",
      resetDefault: "Restore default",
      resetDefaultTitle: "Revert to default (on a credential field, this clears it)",
      test: "Test",
      sending: "sending…",
      testEnabledTitle: "Send a test message through this channel",
      testDisabledTitle: "Fill in credentials to test",
    },
    confirm: {
      clusterScope: "Applies to ALL Nodes without their own value on each Agent's next heartbeat, with no restart required.",
      changeCluster: "Change \"{label}\" to {value}{unit} cluster-wide?",
      resetCluster: "Restore \"{label}\" to the default ({defaultValue}) cluster-wide?",
      restore: "Restore",
    },
    messages: {
      applied: "Applied",
      saveFailed: "Failed to save",
      loadFailed: "Failed to load settings",
      testSent: "Sent — check the channel",
      sendFailed: "Failed to send",
    },
  },
  specs: {
    admission: {
      interval: {
        label: "Evaluation interval",
        description: "Time between admission controller runs. Lower = reacts faster to changes; higher = less load on the database/Cluster.",
      },
      max_cluster_downloads: {
        label: "Download limit (fallback)",
        description: "Maximum torrents downloading in the Cluster when no tracker has reported an L. 0 disables the fallback (no limit).",
      },
      dl_slot_limit_ttl: {
        label: "Observed L validity",
        description: "How long an observed L (slot limit) from a tracker message remains valid before expiring.",
      },
      min_dl_rate: {
        label: "Minimum download rate",
        description: "An active download sustained below this rate is considered slow and yields its slot during healthy-rate rotation. 0 disables slow rotation.",
      },
      disk_min_free: {
        label: "Free disk margin",
        description: "Disk space never committed on a Node. A torrent only enters or migrates to a Node if the projected free space (current free minus what existing torrents still need to download) fits the torrent's TOTAL SIZE plus this margin. Below it, the Node triggers the \"disk full\" alert. Default 5 GiB.",
      },
      slow_grace: {
        label: "Slow grace period",
        description: "Continuous time below the minimum rate before rotating out a slow download. Prevents rotation due to brief dips. Minimum 10min regardless of the configured rate — initial real peer discovery/negotiation can legitimately consume most of a short window. To disable slow rotation, use admission.min_dl_rate=0 (not this field).",
      },
      slow_cooldown: {
        label: "Slow cooldown",
        description: "How long a download rotated for being slow stays out of the promotion queue, preventing immediate re-entry and thrashing.",
      },
      dead_grace: {
        label: "Dead torrent grace period",
        description: "With continuous 0 seeds AND 0 peers for this duration, the torrent is marked dead (will never complete) and yields its slot. Minimum 5min (below that causes false positives — real announce/peer discovery can legitimately take longer); 0 disables dead rotation.",
      },
      dead_cooldown: {
        label: "Dead torrent cooldown",
        description: "How long a rotated dead torrent stays at the back of the queue — it won't be promoted again even with the highest progress.",
      },
      dead_backoff_max: {
        label: "Dead torrent backoff cap",
        description: "Each consecutive time the same torrent is rotated for being dead, the cooldown doubles (30m → 1h → 2h…) up to this cap — a chronically dead torrent backs itself out of the queue instead of returning every cooldown. Connecting to any peer resets the count. 0 disables backoff (cooldown stays fixed).",
      },
      priority_persistent: {
        label: "Persistent priority mode",
        description: "When enabled, a manually prioritized torrent (⚡ button) is NOT rotated for being dead (no seeds/peers) or slow — it holds the slot until the download finishes. Disabled (default): priority only skips the queue; once admitted, the torrent follows normal flow and loses priority if dead/slow rotation demotes it. Warning: in persistent mode, a prioritized torrent with no seeds occupies a slot without downloading until you remove it or seeds appear.",
      },
    },
    agent: {
      artifacts_url: {
        label: "Agent artifacts URL",
        description: "Where the Portal publishes Agent binaries and the agents.json manifest (e.g., http://controller.example/node). The Control Plane reads the manifest here to check available versions, and the Agent DOWNLOADS the binary from this same URL — it must be reachable from the Seed Nodes, not an internal address. Empty disables orchestrated updates (no update badge; the button is refused).",
      },
      heartbeat_interval: {
        label: "Heartbeat interval",
        description: "How often (in seconds) the Agent reports to the Control Plane that it's alive, sends resource stats (and receives this config). Lower = the Portal and alerts see the Node sooner, at the cost of more requests. Keep well below \"Node offline after\" (alerting.node_offline_after, 90s): above half of it, a single missed heartbeat may mark the Node offline.",
      },
      command_poll_interval: {
        label: "Command poll interval",
        description: "How often (in seconds) the Agent asks the Control Plane for commands to run (pause, migrate, update…). This is the latency from an operator action reaching the Node.",
      },
      log_level: {
        label: "Agent log level",
        description: "Verbosity of the Agent log on the Node. `debug` helps troubleshoot a problematic Node (a per-Node override avoids cluttering the entire fleet's logs) but generates a lot of text.",
      },
    },
    engine: {
      queueing_enabled: {
        label: "Engine queue",
        description: "Enables the Engine queue, which limits how many torrents are active at once. This protects against I/O saturation: without a queue, adding many torrents starts all downloads together (I/O at 100%, torrents stuck verifying, and USB HDDs dropping due to power starvation). When disabled, the «engine.max_active_*» limits below no longer apply.",
      },
      max_active_downloads: {
        label: "Simultaneous downloads per Node",
        description: "Maximum downloads the Engine keeps active at once on a Node; the rest wait in the queue. Each concurrent download costs piece hashing and I/O: weaker hardware (Atom, USB HDD) needs a lower number than the fleet — set it with an override on that Node, don't lower the Cluster value. The admission controller enforces the tracker slot limit across the whole Cluster; this is the per-machine safety net.",
      },
      max_active_uploads: {
        label: "Simultaneous uploads per Node",
        description: "Maximum torrents seeding at once. The default is intentionally near-unlimited: a queued seeder (queuedUP) sits idle, sends event=stopped to the tracker, stops accumulating seedtime, and gets Hit and Run. Seeders should never be queued.",
      },
      max_active_torrents: {
        label: "Total active torrents per Node",
        description: "Maximum active torrents (downloading + seeding). For the same reason as uploads, the default is near-unlimited: only downloads and checks should be queued.",
      },
      max_active_checking: {
        label: "Simultaneous checks per Node",
        description: "How many torrents the Engine verifies (piece hashing) at once. Verification reads the torrent's entire data from disk: more than one at a time saturates USB HDD I/O.",
      },
      dont_count_slow: {
        label: "Slow downloads don't count toward ceiling",
        description: "OFF (default) ON PURPOSE: when on, a stalled download (0 KB/s, waiting for a tracker slot) doesn't count toward the ceiling, so the Engine starts ALL downloads, all announce at once, the tracker refuses slots (thundering herd), and everything locks up. When off, the ceiling holds and the excess queues without announcing.",
      },
      slow_dl_threshold: {
        label: "Slow download threshold",
        description: "Below this download speed, a torrent is considered slow by the Engine. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
      },
      slow_ul_threshold: {
        label: "Slow upload threshold",
        description: "Below this upload speed, a torrent is considered slow by the Engine. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
      },
      slow_inactive_timer: {
        label: "Slow time before freeing slot",
        description: "How long continuously below the threshold the Engine waits before treating the torrent as slow. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
      },
    },
    integrations: {
      tmdb_api_key: {
        label: "TMDB — API key (v3)",
        description: "Your TMDB (themoviedb.org) v3 API key to enrich metadata on publish. Empty disables enrichment. This is YOUR key — register at themoviedb.org.",
      },
      igdb_client_id: {
        label: "IGDB — Client ID (Twitch)",
        description: "Twitch app Client ID used by IGDB (game metadata). Without the ID+Secret pair, game publishing is disabled.",
      },
      igdb_client_secret: {
        label: "IGDB — Client Secret (Twitch)",
        description: "Twitch app Client Secret (client-credentials, machine-to-machine). Never shown again.",
      },
      imgbb_api_key: {
        label: "imgbb — API key",
        description: "imgbb.com API key to host publish screenshots. Create yours free at api.imgbb.com. Empty disables screenshot generation.",
      },
      google_books_api_key: {
        label: "Google Books — API key",
        description: "Google Books API key, used as the SECOND book/audiobook metadata provider in the catalog — Open Library doesn't index much of the PT-BR collection or recent releases. PRACTICALLY REQUIRED: without a key the API returns 429 (Too Many Requests) after very few lookups, and a single catalog page already exceeds that — measured, not estimated. Empty = the provider stays idle and the book relies only on what Open Library confirms. Create yours at console.cloud.google.com (enable the Books API).",
      },
    },
    notify: {
      telegram_bot_token: {
        label: "Telegram — bot token",
        description: "Bot token created via @BotFather (format 123456:ABC-DEF…). Filling it enables the channel; restoring the default disables it. Never shown again.",
      },
      telegram_chat_id: {
        label: "Telegram — destination chat_id",
        description: "Where to send (your user, a group, or a channel). A number, possibly negative for groups (e.g., -1001234567890). The channel only sends when both token AND chat_id are filled.",
      },
      discord_webhook_url: {
        label: "Discord — webhook URL",
        description: "Channel webhook URL (Edit Channel → Integrations → Webhooks). Filling it enables the channel. Never shown again.",
      },
      webhook_url: {
        label: "Generic webhook — URL",
        description: "Receives a raw JSON POST on every notification (fields: kind, severity, title, message, node, at). Works with Slack, n8n, Home Assistant, or anything that accepts HTTP — no specific SeedGrid support needed. Never shown again.",
      },
      alerts_min_severity: {
        label: "Alerts — minimum severity",
        description: "Which alerts (Node offline, storage, disk saturated) get notified. OFF disables the alert pipeline without touching channels. CRITICAL = only what needs immediate action; WARNING includes degradation; INFO sends everything.",
      },
      alerts_on_resolve: {
        label: "Alerts — notify on resolve",
        description: "Also sends when an alert resolves (Node came back, storage recovered). Knowing it's back usually matters as much as knowing it went down — and saves pointless checking.",
      },
      events: {
        label: "Events to notify",
        description: "Everyday cluster events. Check what you want to receive; with nothing checked, none are sent.",
      },
      activities: {
        label: "Orchestration decisions to notify",
        description: "What SeedGrid does on its own to balance downloads. ⚠️ Usually happens many times per hour — leave unchecked unless you want to follow closely.",
      },
      language: {
        label: "Notification language",
        description: "Language for channel messages (Telegram, Discord, webhook). It's per cluster, not per operator: notifications go to a channel. The Portal follows each user's language.",
      },
      rate_limit_per_min: {
        label: "Messages per minute cap",
        description: "Protects against flooding (e.g., a Node flapping between ONLINE/OFFLINE). Excess within the window is dropped and the suppressed total goes to the log. 0 disables the limit.",
      },
    },
    alerting: {
      worker_interval: {
        label: "Alert Worker interval",
        description: "Period between evaluations of deterministic alert rules.",
      },
      node_offline_after: {
        label: "Node offline after",
        description: "Time without a heartbeat before a Seed Node is considered offline (triggers alert).",
      },
      disk_saturated_pct: {
        label: "Disk saturation",
        description: "Disk I/O utilization percentage above which the disk is considered saturated (triggers alert).",
      },
      disk_saturated_for: {
        label: "Sustained disk saturation",
        description: "How long the disk must stay continuously above the saturation threshold before raising DISK_IO_SATURATED. Readings come from the heartbeat (~10s) and fluctuate; requiring continuous time avoids alerts from second-long spikes. An Alert Worker check below the threshold resets the counter; the alert fires on the first check after the time is met. The counter restarts if the Controller restarts. Resolution has no grace period. 0 = alert on first reading.",
      },
      storage_grace: {
        label: "Failed storage grace",
        description: "How long a Node's /data must fail before alerting (NODE_STORAGE). The heartbeat is 30s and the alert used to fire on ONE reading: an I/O hiccup—expected on underpowered USB HDDs—became a CRITICAL alert resolved 30 seconds later. Resolution has NO grace period: storage coming back is good news. 0 alerts on first reading, as before.",
      },
      firewalled_grace: {
        label: "Connectability grace",
        description: "How long a Node can receive no incoming connections before alerting (NODE_FIREWALLED). Every Node starts 'firewalled' until the first peer arrives, so alerting immediately would trigger on every Engine restart. 0 disables the rule.",
      },
      incoming_silence: {
        label: "Allowed inbound silence",
        description: "How long a Node can go with NO incoming connections when there is demand before alerting (NODE_INBOUND_SILENT). Unlike connectability grace: that uses the Engine verdict, which never reverses; this uses a decaying signal, catching Nodes that lose reachability in steady state. Incoming connections are rare in an idle collection: a healthy Node may go days without one, and a low value causes false alerts. What counts as demand is defined by \"Seeds per leecher that count as demand\". 0 disables the rule.",
      },
      incoming_demand_seeds_per_leecher: {
        label: "Seeds per leecher that count as demand",
        description: "A torrent only counts as demand for NODE_INBOUND_SILENT when the swarm has at most this many seeds per leecher (seeds <= leechers x value). In a well-seeded swarm, a leecher rarely connects to a specific Node, so no inbound traffic says nothing about the port. 1 counts only swarms where leechers are at least as numerous as seeds; 0 counts only swarms with no seeds; a very high value (e.g., 100000) counts any leecher again. Torrents missing seed or leecher counts don't count.",
      },
      ipv6_change_grace: {
        label: "IPv6 change grace",
        description: "How long a new IPv6 address must persist before alerting (NODE_IPV6_CHANGED). Privacy extensions rotate the suffix automatically; without grace, a misconfigured Node would alert on every ephemeral change. 0 disables the rule and closes any open alerts for it.",
      },
      reachability_ttl: {
        label: "Reachability alert TTL",
        description: "How long NODE_FIREWALLED, NODE_INBOUND_SILENT and NODE_IPV6_CHANGED stay open before auto-closing. All three infer reachability from the ABSENCE of incoming connections and only resolve when one arrives—which a Node with no demand never gets, even with an open port. The notice fires on open; on expiry, the alert closes silently and won't reopen while the condition persists (same IPv6 change, same silence). Counted from the last notification. 0 = no TTL: only closes when an incoming connection arrives.",
      },
      sample_retention: {
        label: "Sample retention",
        description: "How long metric samples are kept before the worker cleans them up.",
      },
    },
    commands: {
      redispatch_after: {
        label: "Orphan command redelivery",
        description: "Age after which a DISPATCHED command with no Agent confirmation is redelivered on the next poll (the Agent died between claiming it and reporting the result). Must be GREATER than the longest expected command: the Agent runs the batch serially and a large migrate_pull holds up the queue while copying data. 0 disables redelivery.",
      },
      max_attempts: {
        label: "Max command deliveries",
        description: "How many Agent deliveries a command can receive before being marked FAILED as abandoned — prevents infinite redelivery loops for commands that always crash the Agent.",
      },
    },
    workers: {
      maintenance_interval: {
        label: "Maintenance Worker interval",
        description: "Interval for retention pruning: terminal commands, events, activities, audit, and expired sessions.",
      },
      reconciler_interval: {
        label: "Migration Reconciler interval",
        description: "Interval for the reconciler that advances/finalizes in-flight torrent migrations.",
      },
      prom_writer_interval: {
        label: "Prometheus Targets Writer interval",
        description: "Rewrite period for the Prometheus targets file (file_sd) with active Nodes.",
      },
    },
    retention: {
      commands: {
        label: "Command retention",
        description: "How long DONE/FAILED commands stay in the database before pruning. PENDING/DISPATCHED are never pruned. 0 disables.",
      },
      events: {
        label: "Event retention",
        description: "How long events stay in the database before pruning. 0 disables.",
      },
      activities: {
        label: "Activity feed retention",
        description: "How long the orchestration trail stays in the database before pruning. 0 disables.",
      },
      audit: {
        label: "Audit retention",
        description: "How long the audit trail stays in the database before pruning. Longer retention for accountability. 0 disables.",
      },
    },
    migration: {
      max_concurrent: {
        label: "Concurrent migrations",
        description: "How many torrent migrations can transfer data at once. Others wait in queue. Keep low: each migration is a heavy disk copy, and draining an entire Node at once saturates the I/O of the very machine in trouble.",
      },
      recheck_stall_window: {
        label: "Migration recheck stall",
        description: "How long the recheck on the target Node can make no progress before the migration is aborted as stalled. This is what matters: while progress increases, waiting continues regardless of disk speed—only a stall fails it. Monitored by the Control Plane on each reconciler pass.",
      },
      recheck_max_wait: {
        label: "Migration recheck ceiling",
        description: "Absolute ceiling, counted from copy completion, for the destination to verify it has what the source had. Only catches an Engine reporting oscillating progress forever; stagnation (above) is the real safeguard. Since the wait no longer blocks the Node command queue, the ceiling can be generous — the cost of a high ceiling is that a migration.max_concurrent slot stays occupied.",
      },
    },
    metrics: {
      torrent_sample_interval: {
        label: "Per-torrent sample interval",
        description: "Minimum interval between time-series sample batches per Node. The Agent syncs every ~15s; writing a sample on every sync is the biggest source of database growth. 0 = sample on every sync.",
      },
      cluster_sample_interval: {
        label: "Cluster sample interval",
        description: "Period between snapshots of cumulative cluster throughput (download/upload) recorded for the Metrics page trend chart (F2). Lower = smoother curve and more rows in the database.",
      },
      cluster_sample_retention: {
        label: "Cluster sample retention",
        description: "How long cluster throughput snapshots are kept. 0 = keep forever (the table is tiny, ~1 row per interval). For the 1-year filter to work, keep ≥ 1 year or 0.",
      },
    },
    publish: {
      screenshot_count: {
        label: "Screenshots per upload",
        description: "Number of frames to extract from the video file when generating screenshots for publishing to a tracker.",
      },
    },
  },
  channelSpecs: {
    custom: {
      label: "{channel} — custom filters",
      description: "When enabled, this channel uses the filters below instead of the global ones. When disabled (default), it follows the global notification filters.",
    },
    alerts_min_severity: {
      label: "{channel} — minimum alert severity",
      description: "Only applies when custom filters are enabled. OFF sends no alerts on this channel.",
    },
    alerts_on_resolve: {
      label: "{channel} — notify on alert resolve",
      description: "Only applies when custom filters are enabled.",
    },
    events: {
      label: "{channel} — events",
      description: "Only applies when custom filters are enabled. If nothing is selected, no events are sent on this channel.",
    },
    activities: {
      label: "{channel} — orchestration decisions",
      description: "Only applies when custom filters are enabled. If nothing is selected, none are sent on this channel.",
    },
    rate_limit_per_min: {
      label: "{channel} — messages per minute cap",
      description: "Only applies when custom filters are enabled. 0 = no cap — use 0 on a channel that triggers automation so a burst of notices doesn't drop the trigger. ⚠️ ALERTS never count toward this cap on any channel: alerts exist to be intrusive.",
    },
  },
  choices: {
    event: {
      NODE_REGISTERED: "Node joined the Cluster",
      NODE_DELETED: "Node removed",
      TORRENT_CREATED: "Torrent added",
      TORRENT_REMOVED: "Torrent removed",
      TORRENT_PAUSED: "Torrent paused",
      TORRENT_RESUMED: "Torrent resumed",
      TORRENT_RECHECK: "Torrent rechecking",
      TORRENT_MIGRATION_STARTED: "Torrent migration started",
      TORRENT_MIGRATED: "Torrent moved to another Node",
      TORRENT_MIGRATION_FAILED: "Torrent migration failed",
      TORRENT_RESEEDED: "Torrent reseeded",
      TORRENT_PUBLISHED_EXTERNAL: "Torrent published to tracker",
      ENGINE_RESTART: "Engine restarted",
      AGENT_UPDATE: "Agent updated",
      STORAGE_RECOVERY: "Node disk reconnected",
    },
    activity: {
      admission_hold: "Download paused to free a slot",
      admission_resume: "Download resumed when slot opened",
      scheduled: "Torrent assigned to Node",
      downloading: "Torrent started downloading",
      seeding: "Torrent started seeding",
      node_blocked: "Torrent could not be scheduled (Node disk failing)",
      screenshot_host_failed: "Image host failed to upload screenshot",
    },
  },
  categoryNames: {
    admission: "Admission",
    agent: "Agent / Engine",
    alerting: "Alerts",
    notify: "Notifications",
    integrations: "Integrations",
    workers: "Workers",
    publish: "Publishing",
  },
};
