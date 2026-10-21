package alertgroup_test

import (
	"fmt"
	"testing"
	"time"

	alertgroup "github.com/yornik/go-practice/exercises/44-alertgroup"
)

// Step 4: inhibition. Run with -run 'TestInhibit|TestIncident'.
// All scenarios use defaultRoute: GroupWait 30s, GroupInterval 5m,
// RepeatInterval 4h, one group per alertname.

// nodeDownRule mutes the warnings on a node while that node is down.
func nodeDownRule() alertgroup.InhibitRule {
	return alertgroup.InhibitRule{
		SourceMatch: ls("alertname=NodeDown"),
		TargetMatch: ls("severity=warning"),
		Equal:       []string{"node"},
	}
}

func TestInhibit(t *testing.T) {
	tests := []struct {
		name  string
		rules []alertgroup.InhibitRule
		steps []step
	}{
		{
			// Rule 15. The source mutes targets in other groups too. db-0 is
			// critical, so TargetMatch does not match it; web-0 is on another
			// node, so Equal does not hold.
			name:  "the cause mutes the symptoms on its node",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
					firing("alertname=PodNotReady node=n1 pod=db-0 severity=critical"),
					firing("alertname=PodNotReady node=n2 pod=web-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
					`{alertname="PodNotReady"} firing=[node=n1,pod=db-0,severity=critical node=n2,pod=web-0,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rule 15. Neither NodeDown nor api-0 has a cluster label, so both
			// have cluster "" and Equal holds. This is a classic
			// Alertmanager surprise: a rule with equal: [cluster] mutes
			// everything when the alerts carry no cluster label.
			name: "a label missing on both sides is equal",
			rules: []alertgroup.InhibitRule{{
				SourceMatch: ls("alertname=NodeDown"),
				TargetMatch: ls("severity=warning"),
				Equal:       []string{"cluster"},
			}},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=PodNotReady node=n2 pod=api-0 severity=warning"),
					firing("alertname=PodNotReady cluster=eu1 node=n1 pod=web-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
					`{alertname="PodNotReady"} firing=[cluster=eu1,node=n1,pod=web-0,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rules 11 and 16. The PodNotReady group never notified: all its
			// alerts were muted. When NodeDown resolves, api-0 is firing
			// again and its group notifies at once (GroupWait is long over).
			name:  "the symptoms show up when the cause resolves",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 10 * minute, receive: []alertgroup.Alert{
					endsAt(10*minute, "alertname=NodeDown node=n1 severity=critical"),
				}, want: []string{
					`{alertname="NodeDown"} firing=[] resolved=[node=n1,severity=critical]`,
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rules 3 and 15. A source with EndsAt in the future is firing
			// and mutes. At its EndsAt it resolves by itself and mutes
			// nothing any more.
			name:  "a source mutes only while it is firing",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					endsAt(2*minute, "alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 1 * minute},
				{at: 2 * minute, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning] resolved=[]`,
				}},
				{at: 5*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[] resolved=[node=n1,severity=critical]`,
				}},
			},
		},
		{
			// Rules 12 and 17. api-0 was notified, then muted. Muting is not
			// a change, so nothing is sent at 5m30s. When api-0 resolves,
			// the receiver hears about it: resolved alerts are never muted.
			name:  "muting is not a change, and a muted alert that resolves is reported",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
					firing("alertname=PodNotReady node=n2 pod=web-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning node=n2,pod=web-0,severity=warning] resolved=[]`,
				}},
				{at: 1 * minute, receive: []alertgroup.Alert{firing("alertname=NodeDown node=n1 severity=critical")}},
				{at: 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 5*minute + 30*second},
				{at: 6 * minute, receive: []alertgroup.Alert{
					endsAt(6*minute, "alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
				}, want: []string{
					`{alertname="PodNotReady"} firing=[node=n2,pod=web-0,severity=warning] resolved=[node=n1,pod=api-0,severity=warning]`,
				}},
			},
		},
		{
			// Rules 12, 13 and 16. One alert gets muted and a new one
			// arrives: the number of firing alerts is the same, but the set
			// is not. Later the muted alert comes back. The last
			// notification did not list it, so that is a change too, even
			// though an older notification did.
			name:  "changes are judged against the last notification",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
					firing("alertname=PodNotReady node=n2 pod=web-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning node=n2,pod=web-0,severity=warning] resolved=[]`,
				}},
				{at: 1 * minute, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=PodNotReady node=n3 pod=cache-0 severity=warning"),
				}},
				{at: 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 5*minute + 30*second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n2,pod=web-0,severity=warning node=n3,pod=cache-0,severity=warning] resolved=[]`,
				}},
				// n1 is back, but api-0 is still not ready.
				{at: 6 * minute, receive: []alertgroup.Alert{
					endsAt(6*minute, "alertname=NodeDown node=n1 severity=critical"),
				}},
				{at: 6*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[] resolved=[node=n1,severity=critical]`,
				}},
				{at: 10*minute + 29*second},
				{at: 10*minute + 30*second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning node=n2,pod=web-0,severity=warning node=n3,pod=cache-0,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rule 12, second case. The reminder is about what is in Firing.
			// When every alert of a group is muted, there is nothing to
			// remind anybody of.
			name:  "a group whose alerts are all muted sends no reminder",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning] resolved=[]`,
				}},
				{at: 1 * minute, receive: []alertgroup.Alert{firing("alertname=NodeDown node=n1 severity=critical")}},
				{at: 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 4*hour + 30*second},
				{at: 4*hour + 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
			},
		},
		{
			// Rule 15, "for at least one rule". Many rule sets fire the same
			// alert as a warning first and as critical later, and mute the
			// warning while the critical one fires. The n1 warning matches
			// the TargetMatch of the NodeDown rule too, which has no source;
			// the second rule still mutes it.
			name: "every rule is tried",
			rules: []alertgroup.InhibitRule{
				nodeDownRule(),
				{
					SourceMatch: ls("severity=critical"),
					TargetMatch: ls("severity=warning"),
					Equal:       []string{"alertname", "node"},
				},
			},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=DiskFull node=n1 severity=warning"),
					firing("alertname=DiskFull node=n1 severity=critical"),
					firing("alertname=DiskFull node=n2 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="DiskFull"} firing=[node=n1,severity=critical node=n2,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rule 15, last condition: it needs both halves. The warning
			// NodeDown matches SourceMatch and TargetMatch. The critical one
			// matches only SourceMatch, so it counts as a source and mutes
			// the warning on its node. The two warnings match both sides,
			// so they do not count as sources for each other.
			name:  "a target that matches both sides is muted by a source that matches one",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 severity=warning"),
					firing("alertname=NodeDown node=n1 severity=critical"),
					firing("alertname=NodeDown node=n2 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical node=n2,severity=warning] resolved=[]`,
				}},
			},
		},
		{
			// Rules 8 and 12. The repeat is a full snapshot of what is
			// firing and not muted now.
			name:  "a repeat leaves out muted alerts",
			rules: []alertgroup.InhibitRule{nodeDownRule()},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=PodNotReady node=n1 pod=api-0 severity=warning"),
					firing("alertname=PodNotReady node=n2 pod=web-0 severity=warning"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n1,pod=api-0,severity=warning node=n2,pod=web-0,severity=warning] resolved=[]`,
				}},
				{at: 1 * minute, receive: []alertgroup.Alert{firing("alertname=NodeDown node=n1 severity=critical")}},
				{at: 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
				{at: 4*hour + 30*second, want: []string{
					`{alertname="PodNotReady"} firing=[node=n2,pod=web-0,severity=warning] resolved=[]`,
				}},
				{at: 4*hour + 1*minute + 30*second, want: []string{
					`{alertname="NodeDown"} firing=[node=n1,severity=critical] resolved=[]`,
				}},
			},
		},
		{
			// Rule 15, last condition. "NodeDown mutes every other alert on
			// its node": the empty TargetMatch matches NodeDown too. Without
			// the condition, each NodeDown would mute itself, and the two
			// NodeDowns (one per Prometheus server) would mute each other.
			name: "an alert does not mute itself, and alerts that match both sides do not mute each other",
			rules: []alertgroup.InhibitRule{{
				SourceMatch: ls("alertname=NodeDown"),
				TargetMatch: nil, // matches every alert
				Equal:       []string{"node"},
			}},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=NodeDown node=n1 prometheus=a"),
					firing("alertname=NodeDown node=n1 prometheus=b"),
					firing("alertname=KubeletDown node=n1"),
					firing("alertname=KubeletDown node=n2"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="KubeletDown"} firing=[node=n2] resolved=[]`,
					`{alertname="NodeDown"} firing=[node=n1,prometheus=a node=n1,prometheus=b] resolved=[]`,
				}},
			},
		},
		{
			// Rule 15, first condition. NodeDown is muted by
			// ClusterUnreachable, and still mutes api-0. A source counts
			// whether it is muted or not, so the order in which you check
			// the alerts cannot change the result.
			name: "a muted alert still mutes others",
			rules: []alertgroup.InhibitRule{
				{
					SourceMatch: ls("alertname=ClusterUnreachable"),
					TargetMatch: ls("alertname=NodeDown"),
					Equal:       []string{"cluster"},
				},
				{
					SourceMatch: ls("alertname=NodeDown"),
					TargetMatch: ls("alertname=PodNotReady"),
					Equal:       []string{"node"},
				},
			},
			steps: []step{
				{at: 0, receive: []alertgroup.Alert{
					firing("alertname=PodNotReady cluster=eu1 node=n1 pod=api-0"),
					firing("alertname=PodNotReady cluster=eu1 node=n2 pod=web-0"),
					firing("alertname=NodeDown cluster=eu1 node=n1"),
					firing("alertname=ClusterUnreachable cluster=eu1"),
				}},
				{at: 30 * second, want: []string{
					`{alertname="ClusterUnreachable"} firing=[cluster=eu1] resolved=[]`,
					`{alertname="PodNotReady"} firing=[cluster=eu1,node=n2,pod=web-0] resolved=[]`,
				}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run(t, defaultRoute(), tc.rules, tc.steps)
		})
	}
}

// TestIncident is the story from the README: one node goes down and 40 alerts
// fire. With grouping, dedup, GroupWait and one inhibition rule, the on-call
// engineer gets 2 messages about the node instead of 40 pages every minute.
func TestIncident(t *testing.T) {
	route := defaultRoute()
	route.GroupBy = []string{"alertname", "cluster"}
	rules := []alertgroup.InhibitRule{{
		SourceMatch: ls("alertname=NodeDown"),
		TargetMatch: ls("severity=warning"),
		Equal:       []string{"cluster", "node"},
	}}

	// 39 symptoms on node n1: 30 pods that are not ready and 9 scrape
	// targets that are down.
	var symptoms []string
	for i := range 30 {
		symptoms = append(symptoms, fmt.Sprintf("alertname=KubePodNotReady cluster=prod namespace=shop node=n1 pod=api-%02d severity=warning", i))
	}
	for _, job := range []string{"node-exporter", "kubelet", "cadvisor", "kube-proxy", "calico-node",
		"fluent-bit", "csi-node", "ingress-nginx", "otel-agent"} {
		symptoms = append(symptoms, "alertname=TargetDown cluster=prod job="+job+" node=n1 severity=warning")
	}
	const (
		cause     = "alertname=NodeDown cluster=prod node=n1 severity=critical"
		unrelated = "alertname=KubePodNotReady cluster=prod namespace=shop node=n2 pod=web-0 severity=warning"
	)
	// all builds one alert per label string.
	all := func(build func(string) alertgroup.Alert, labels ...string) []alertgroup.Alert {
		var alerts []alertgroup.Alert
		for _, l := range labels {
			alerts = append(alerts, build(l))
		}
		return alerts
	}
	resolvedAt := func(d time.Duration) func(string) alertgroup.Alert {
		return func(l string) alertgroup.Alert { return endsAt(d, l) }
	}
	everything := append([]string{cause, unrelated}, symptoms...)

	run(t, route, rules, []step{
		// The pods and targets fail first. The node's heartbeat times out
		// 15 seconds later. GroupWait is why that is soon enough: the
		// symptom groups wait 30s before they say anything.
		{at: 0, receive: all(firing, append([]string{unrelated}, symptoms...)...)},
		{at: 15 * second, receive: all(firing, cause)},
		// Prometheus keeps re-sending every firing alert.
		{at: 30 * second, receive: all(firing, everything...), want: []string{
			`{alertname="KubePodNotReady", cluster="prod"} firing=[namespace=shop,node=n2,pod=web-0,severity=warning] resolved=[]`,
		}},
		{at: 45 * second, want: []string{
			`{alertname="NodeDown", cluster="prod"} firing=[node=n1,severity=critical] resolved=[]`,
		}},
		{at: 1 * minute, receive: all(firing, everything...)},
		{at: 1*minute + 30*second, receive: all(firing, everything...)},
		// The node is back. Prometheus resolves the cause and the symptoms.
		// Nobody heard of the symptoms, so only the cause is reported.
		{at: 12 * minute, receive: all(resolvedAt(12*minute), append([]string{cause}, symptoms...)...), want: []string{
			`{alertname="NodeDown", cluster="prod"} firing=[] resolved=[node=n1,severity=critical]`,
		}},
		// The unrelated pod is still not ready: a reminder, without the 30
		// pods of n1.
		{at: 4*hour + 30*second, want: []string{
			`{alertname="KubePodNotReady", cluster="prod"} firing=[namespace=shop,node=n2,pod=web-0,severity=warning] resolved=[]`,
		}},
	})
}
