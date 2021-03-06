package ptest

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/elastic/beats/metricbeat/mb"
	mbtest "github.com/elastic/beats/metricbeat/mb/testing"

	"github.com/stretchr/testify/assert"
)

var expectedFlag = flag.Bool("update_expected", false, "Update prometheus expected files")

// TestCases holds the list of test cases to test a metricset
type TestCases []struct {
	// MetricsFile containing Prometheus outputted metrics
	MetricsFile string

	// ExpectedFile containing resulting documents
	ExpectedFile string
}

// TestMetricSet goes over the given TestCases and ensures that source Prometheus metrics gets converted into the expected
// events when passed by the given metricset.
// If -update_expected flag is passed, the expected JSON file will be updated with the result
func TestMetricSet(t *testing.T, module, metricset string, cases TestCases) {
	for _, test := range cases {
		t.Logf("Testing %s file\n", test.MetricsFile)

		file, err := os.Open(test.MetricsFile)
		assert.NoError(t, err, "cannot open test file "+test.MetricsFile)

		body, err := ioutil.ReadAll(file)
		assert.NoError(t, err, "cannot read test file "+test.MetricsFile)

		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "text/plain; charset=ISO-8859-1")
			w.Write([]byte(body))
		}))

		server.Start()
		defer server.Close()

		config := map[string]interface{}{
			"module":     module,
			"metricsets": []string{metricset},
			"hosts":      []string{server.URL},
		}

		f := mbtest.NewReportingMetricSetV2(t, config)
		reporter := &mbtest.CapturingReporterV2{}
		f.Fetch(reporter)
		assert.Nil(t, reporter.GetErrors(), "Errors while fetching metrics")

		if *expectedFlag {
			eventsJSON, _ := json.MarshalIndent(reporter.GetEvents(), "", "\t")
			err = ioutil.WriteFile(test.ExpectedFile, eventsJSON, 0644)
			assert.NoError(t, err)
		}

		// Read expected events from reference file
		expected, err := ioutil.ReadFile(test.ExpectedFile)
		if err != nil {
			t.Fatal(err)
		}

		var expectedEvents []mb.Event
		err = json.Unmarshal(expected, &expectedEvents)
		if err != nil {
			t.Fatal(err)
		}

		for _, event := range reporter.GetEvents() {
			// ensure the event is in expected list
			found := -1
			for i, expectedEvent := range expectedEvents {
				if event.RootFields.String() == expectedEvent.RootFields.String() &&
					event.ModuleFields.String() == expectedEvent.ModuleFields.String() &&
					event.MetricSetFields.String() == expectedEvent.MetricSetFields.String() {
					found = i
					break
				}
			}
			if found > -1 {
				expectedEvents = append(expectedEvents[:found], expectedEvents[found+1:]...)
			} else {
				t.Errorf("Event was not expected: %+v", event)
			}
		}

		if len(expectedEvents) > 0 {
			t.Error("Some events were missing:")
			for _, e := range expectedEvents {
				t.Error(e)
			}
			t.Fatal()
		}
	}
}
