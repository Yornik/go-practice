# 19 · Alert dedup, grouping and inhibition

**Tier 4 · about 4 h** · Go: canonical keys from maps (sorted keys), `hash/fnv`, map aliasing and `maps.Clone`, `slices.SortFunc`, `time.Time` comparisons, a state machine driven by explicit timestamps · Problem solving: turn prose rules into a table of scenarios; simulate time by hand

At 03:00 one Kubernetes node loses its network. Prometheus fires `NodeDown` for it, and around the
same time 30 `KubePodNotReady` and 9 `TargetDown` alerts, all caused by that one node. It re-sends
every firing alert every minute or two. If each of those became a page, the on-call engineer
would get 40 pages, and then the same 40 again at every re-send. Alertmanager sits between
Prometheus and the pager to stop that: it **deduplicates** re-sent alerts, **groups** related alerts
into one notification, decides **when** a group may notify again, and **inhibits** (mutes) the
symptoms while their cause is firing. Every reliability engineer configures `group_by`,
`group_wait`, `group_interval`, `repeat_interval` and `inhibit_rules`, and most have been surprised
by them at least once. You build that core as a deterministic engine: no goroutines, no timers.
Every call gets the current time as an argument, so a test plays 8 hours in a microsecond, and
you can play them on paper.

## Task

Implement the stubs in `labels.go` and `alertgroup.go`:

```go
type Labels map[string]string
func (l Labels) String() string      // {alertname="NodeDown", node="n1"}: sorted, values quoted
func (l Labels) Fingerprint() uint64 // FNV-1a 64 of the sorted labels: the alert's identity

type Alert struct {
	Labels   Labels
	StartsAt time.Time
	EndsAt   time.Time // zero: firing until further notice
}
func (a Alert) ResolvedAt(now time.Time) bool // EndsAt is set and not after now

type Route struct {
	GroupBy        []string      // label names that form a group
	GroupWait      time.Duration // a new group waits this long before its first notification
	GroupInterval  time.Duration // minimum gap before a notification about changes
	RepeatInterval time.Duration // gap before an unchanged group is repeated
}
type InhibitRule struct {
	SourceMatch Labels   // the cause, e.g. {alertname="NodeDown"}
	TargetMatch Labels   // the symptoms, e.g. {severity="warning"}
	Equal       []string // labels that must be equal in source and target, e.g. node
}
type Notification struct {
	GroupKey    string // GroupLabels.String()
	GroupLabels Labels
	Firing      []Alert
	Resolved    []Alert
}

type Dispatcher struct{ /* you design the fields */ }
func New(r Route, rules []InhibitRule) *Dispatcher
func (d *Dispatcher) Receive(a Alert, now time.Time)     // store a new alert or update a known one
func (d *Dispatcher) Flush(now time.Time) []Notification // the notifications due at now
```

In a real server an HTTP handler calls Receive for every alert and a ticker calls Flush every few
seconds. The caller passes non-decreasing times and serializes the calls: a Dispatcher is not safe
for concurrent use.

## Rules

The tests refer to these numbers. The rules are a simplified model of Alertmanager, not a copy
(for example, Alertmanager checks groups on a fixed `GroupInterval` tick). Where they differ, the
tests check the rules here.

Two words look alike. **firing** (plain) means not resolved (rule 3). **`Firing`** (code font)
means the list in rule 8: the firing alerts that are not muted. A muted alert is firing, but it is
not in `Firing`.

### Labels and alerts

1. `String()`: `{`, then `name="value"` for every label sorted by name, separated by `, `, then
   `}`. The value is quoted like `strconv.Quote`: `{msg="say \"hi\""}`. Nil or empty: `{}`.
2. `Fingerprint()`: the FNV-1a 64-bit hash (`hash/fnv`) of this byte stream: for every label sorted
   by name, the name, a byte `0xff`, the value, a byte `0xff`. Nil or empty Labels hash no bytes
   (14695981039346656037). This is Prometheus's `model.LabelSet.Fingerprint`, the alert identity
   in Alertmanager.
3. `ResolvedAt(now)` is true when EndsAt is not zero and EndsAt <= now. A zero EndsAt means firing.
   An EndsAt in the future means firing until then: Prometheus sends EndsAt a few minutes ahead
   as a "valid until", and re-sends the alert to push it further out.

### Receive

4. Alerts with the same fingerprint are the same alert. Receiving a known alert updates it:
   EndsAt becomes the new EndsAt, even an earlier one (zero: firing again), and StartsAt becomes
   the earlier of the two. Nothing else changes: the alert stays in its group and no timer
   restarts. Known means the Dispatcher still holds it: an alert that Flush removed (rules 10 and
   13) is new when it returns.
5. The **group labels** of an alert are its labels whose names are in `GroupBy`. A `GroupBy` label
   that the alert lacks is left out (not set to `""`), so alerts that lack it share a group. Empty
   `GroupBy`: one group for all alerts, `{}`. The group key is `GroupLabels.String()`, so the
   order of `GroupBy` does not matter.
6. If no group has the alert's key, Receive creates one. Its creation time is Receive's `now`, not
   the alert's StartsAt.
7. Receive keeps its own copy of the labels. The caller may change or reuse its map afterwards.

### Flush: what a group contains at now

8. **`Firing`**: alerts that are not resolved at now and not muted (rules 15 to 17).
9. **Resolved**: alerts that are resolved at now and that were in `Firing` of the group's last
   notification. The receiver saw them firing, so it must hear that they are over.
10. Any other resolved alert (it resolved before a notification listed it, or it arrived already
    resolved) is removed silently by Flush, whether the group notifies or not. It never appears
    in a notification and is not a change.

### Flush: when a group notifies

11. A group that has never notified notifies at the first Flush with
    now >= creation + `GroupWait` at which its `Firing` has at least 1 alert. A group whose alerts
    are all muted sends nothing (not even an empty notification) and keeps waiting.
12. A group whose last notification was at `last` notifies when either holds:
    - it **changed** and now >= last + `GroupInterval`. Changed means: an alert in `Firing` that
      was not in `Firing` of the last notification, or at least 1 resolved alert (rule 9). An
      alert that leaves `Firing` because it is muted is not a change, and neither is a new alert
      that is muted.
    - its `Firing` has at least 1 alert and now >= last + `RepeatInterval`, changed or not.
13. A notification is a full snapshot of the group: its `Firing` holds every alert of rule 8,
    and its `Resolved` every alert of rule 9. After it, the resolved alerts are removed from the
    group, and `last` and the alerts in `Firing` replace what the group remembered for rules 9
    and 12.
14. After Flush, a group without alerts is deleted. A later alert with its key creates a new group
    (rule 6) and waits `GroupWait` again.

### Inhibition

15. A firing alert T is **muted** if, for **at least one** rule, a source alert S in the Dispatcher
    (in any group) meets all of these:
    - S is firing at now. Whether S is muted itself does not matter.
    - S matches `SourceMatch` and T matches `TargetMatch`. An alert matches when it has every
      label of the matcher with the same value. A nil or empty matcher matches every alert.
    - S and T have the same value for every label in `Equal`. A missing label counts as `""`, so
      a label that both lack is equal.
    - Not (T matches `SourceMatch` and S matches `TargetMatch`). This keeps an alert from muting
      itself, and two alerts that match both sides of a rule from muting each other.
16. Muting is decided at every Flush from the current state. When S resolves, T is in `Firing`
    again; if the last notification did not list it, that is a change (rule 12).
17. Only firing alerts are muted. A resolved alert (rule 9) is reported even if a source matches it.

### Output

18. Flush returns the notifications sorted by `GroupKey`, and `Firing` and `Resolved` sorted by
    `Labels.String()` (plain string comparison). When nothing is due, it returns an empty or nil
    slice. An empty `Firing` or `Resolved` may be nil.
19. Alerts in a notification carry their full labels and their StartsAt and EndsAt (rule 4). The
    caller owns everything Flush returns: changing it does not change the Dispatcher.

## Examples

`GroupBy: [alertname]`, `GroupWait: 30s`, `GroupInterval: 5m`, `RepeatInterval: 4h`
(the timings are Alertmanager's defaults). Each line is a Receive or a Flush at that time. Flush runs every few
seconds; only the interesting calls are shown.

```text
0s      Receive Disk{node=a}            group {alertname="Disk"} created, waits until 30s
10s     Receive Disk{node=b}            joins the group, the wait does not restart
30s     Flush → {alertname="Disk"} firing=[node=a node=b] resolved=[]
1m      Receive Disk{node=c}            a change, but the last notification was at 30s
2m      Receive Disk{node=a}            re-sent by Prometheus: same alert, not a change
2m      Receive Disk{node=b, EndsAt=2m} b resolved
5m30s   Flush → {alertname="Disk"} firing=[node=a node=c] resolved=[node=b]
10m30s  Flush → nothing                 nothing changed since 5m30s
4h5m30s Flush → {alertname="Disk"} firing=[node=a node=c] resolved=[]   (repeat)
```

A flap: `Disk{node=d}` arrives at 0s and resolves at 10s. Flush at 10s drops it and deletes the
empty group, so nothing is ever sent.

Inhibition with `SourceMatch: {alertname="NodeDown"}`, `TargetMatch: {severity="warning"}`,
`Equal: [node]`:

```text
0s   Receive PodNotReady{node=n1, pod=api-0, severity=warning}
     Receive PodNotReady{node=n2, pod=web-0, severity=warning}
15s  Receive NodeDown{node=n1, severity=critical}
30s  Flush → {alertname="PodNotReady"} firing=[node=n2,pod=web-0,severity=warning] resolved=[]
             (api-0 is muted; the NodeDown group was created at 15s and waits until 45s)
45s  Flush → {alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]
```

`GroupWait` is what makes this work: the pod alerts wait 30 seconds, long enough for the cause to
arrive. With `Equal: [cluster]` and alerts that have no `cluster` label, the same rule would mute
every warning in the whole fleet.

## Run the tests

    go test ./exercises/19-alertgroup/
    go test -run 'TestLabels|TestAlert' -v ./exercises/19-alertgroup/    # step 1: labels.go
    go test -run TestGroup -v ./exercises/19-alertgroup/                 # step 2: grouping, dedup, GroupWait
    go test -run TestTiming -v ./exercises/19-alertgroup/                # step 3: GroupInterval, RepeatInterval, resolved
    go test -run 'TestInhibit|TestIncident' -v ./exercises/19-alertgroup/ # step 4: inhibition
    go test -run 'TestInhibit/muted_alert_still' -v ./exercises/19-alertgroup/   # one scenario

A stub that panics stops the whole test binary, so run only the step you are on. Most tests are
scenarios: "at time X, receive these alerts, then Flush must return exactly these notifications".
`render` in `helpers_test.go` prints each notification as one line, without the `GroupBy` labels.
A failure names the Flush time and shows both lists. Replay the scenario on paper up to that time
to find the rule.

## Docs you can use

- [`hash/fnv`](https://pkg.go.dev/hash/fnv)
- [`maps`](https://pkg.go.dev/maps): `maps.Keys`, `maps.Clone`; [`slices`](https://pkg.go.dev/slices): `slices.Sorted`, `slices.SortFunc`
- [`strconv.Quote`](https://pkg.go.dev/strconv#Quote), [`strings.Builder`](https://pkg.go.dev/strings#Builder), [`strings.Compare`](https://pkg.go.dev/strings#Compare)
- [`time.Time`](https://pkg.go.dev/time#Time): `Add`, `Before`, `After`, `IsZero`, `Equal`
- The Go spec: [Map types](https://go.dev/ref/spec#Map_types), [For statements with range clause](https://go.dev/ref/spec#For_range) (iteration order, deleting while iterating)
- [Go maps in action](https://go.dev/blog/maps)
- Alertmanager docs: [Alertmanager concepts](https://prometheus.io/docs/alerting/latest/alertmanager/) and [Configuration](https://prometheus.io/docs/alerting/latest/configuration/) (`route`, `inhibit_rule`)

## Stretch goals

- Routing tree: several routes with matchers, each with its own `GroupBy` and timings, and
  `continue`. The group key then starts with the route's path.
- The imperative shell: an HTTP handler for `POST /api/v2/alerts` that decodes JSON alerts and
  calls Receive, a ticker that calls Flush and posts notifications to a webhook, and a mutex that
  serializes both. Inject the clock so it stays testable.
- Silences: a list of matchers with a start and end time. A silenced alert is muted like an
  inhibited one. Which of rules 15 to 17 change?
- Persist the "what was last sent" state (Alertmanager calls it the notification log), so that a
  restart does not re-page for every firing group.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 19 hint 1
    go run ./cmd/peek 19 hint 2
    go run ./cmd/peek 19 hint 3
    go run ./cmd/peek 19 explanation
    go run ./cmd/peek 19 walkthrough
    go run ./cmd/peek 19 solution
