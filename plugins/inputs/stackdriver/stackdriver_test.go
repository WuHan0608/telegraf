package stackdriver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/influxdata/telegraf"

	"github.com/googleapis/gax-go"
	"github.com/influxdata/telegraf/internal"
	"github.com/influxdata/telegraf/testutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

type mockGatherStackdriverClient struct{}

func (m *mockGatherStackdriverClient) ListTimeSeries(
	ctx context.Context,
	req *monitoringpb.ListTimeSeriesRequest,
	opts ...gax.CallOption,
) ([]*monitoringpb.TimeSeries, error) {
	timeSeries := make([]*monitoringpb.TimeSeries, 1)

	switch req.Filter {
	case `metric.type = "testing/boolean"`:
		timeSeries[0] = &monitoringpb.TimeSeries{
			Metric: &metric.Metric{
				Type: "testing/boolean",
				Labels: map[string]string{
					"alias": "bool",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "testing",
				Labels: map[string]string{
					"name": "testing",
					"type": "boolean",
				},
			},
			ValueType: metric.MetricDescriptor_BOOL,
			Points: []*monitoringpb.Point{
				&monitoringpb.Point{
					Interval: req.Interval,
					Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_BoolValue{
							BoolValue: true,
						},
					},
				},
			},
		}
	case `metric.type = "testing/double"`:
		timeSeries[0] = &monitoringpb.TimeSeries{
			Metric: &metric.Metric{
				Type: "testing/double",
				Labels: map[string]string{
					"alias": "float64",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "testing",
				Labels: map[string]string{
					"name": "testing",
					"type": "double",
				},
			},
			ValueType: metric.MetricDescriptor_DOUBLE,
			Points: []*monitoringpb.Point{
				&monitoringpb.Point{
					Interval: req.Interval,
					Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_DoubleValue{
							DoubleValue: 1.0,
						},
					},
				},
			},
		}
	case `metric.type = "testing/int64"`:
		timeSeries[0] = &monitoringpb.TimeSeries{
			Metric: &metric.Metric{
				Type: "testing/int64",
				Labels: map[string]string{
					"alias": "integer",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "testing",
				Labels: map[string]string{
					"name": "testing",
					"type": "int64",
				},
			},
			ValueType: metric.MetricDescriptor_INT64,
			Points: []*monitoringpb.Point{
				&monitoringpb.Point{
					Interval: req.Interval,
					Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_Int64Value{
							Int64Value: 1,
						},
					},
				},
			},
		}
	case `metric.type = "testing/string"`:
		timeSeries[0] = &monitoringpb.TimeSeries{
			Metric: &metric.Metric{
				Type: "testing/string",
				Labels: map[string]string{
					"alias": "string",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "testing",
				Labels: map[string]string{
					"name": "testing",
					"type": "string",
				},
			},
			ValueType: metric.MetricDescriptor_STRING,
			Points: []*monitoringpb.Point{
				&monitoringpb.Point{
					Interval: req.Interval,
					Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_StringValue{
							StringValue: "hello",
						},
					},
				},
			},
		}
	case `metric.type = "testing/distribution"`:
		timeSeries[0] = &monitoringpb.TimeSeries{
			Metric: &metric.Metric{
				Type: "testing/distribution",
				Labels: map[string]string{
					"alias": "distribution",
				},
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "testing",
				Labels: map[string]string{
					"name": "testing",
					"type": "distribution",
				},
			},
			ValueType: metric.MetricDescriptor_DISTRIBUTION,
			Points: []*monitoringpb.Point{
				&monitoringpb.Point{
					Interval: req.Interval,
					Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_DistributionValue{
							DistributionValue: &distribution.Distribution{
								Count:                 1,
								Mean:                  1.0,
								SumOfSquaredDeviation: 2.0,
								Range: &distribution.Distribution_Range{
									Min: 1.0,
									Max: 2.0,
								},
							},
						},
					},
				},
			},
		}
	default:
		return nil, errors.New("unknown metric type")
	}

	return timeSeries, nil
}

func TestGather(t *testing.T) {
	var (
		acc    testutil.Accumulator
		tags   map[string]string
		fields map[string]interface{}
	)

	duration, _ := time.ParseDuration("1m")
	internalDuration := internal.Duration{
		Duration: duration,
	}

	req := &TimeSeriesRequest{
		Aggregation: &TimeSeriesAggregation{
			AlignmentPeriod: internalDuration,
		},
	}

	s := &Stackdriver{
		Project:            "boolean",
		Interval:           internalDuration,
		Delay:              internalDuration,
		RateLimit:          1,
		TimeSeriesRequests: []*TimeSeriesRequest{req},
		client:             new(mockGatherStackdriverClient),
	}

	// error
	var acc1 telegraf.Accumulator
	assert.Equal(t, errors.New("metric_types is a required field for stackdriver input"), s.gatherTimeSeries(acc1, req))

	req.MetricTypes = []string{"testing/none"}
	assert.Equal(t, errors.New("unknown metric type"), s.gatherTimeSeries(acc1, req))

	// boolean
	req.MetricTypes = []string{"testing/boolean"}
	acc.GatherError(s.Gather)

	tags = map[string]string{
		"name":  "testing",
		"type":  "boolean",
		"alias": "bool",
	}
	fields = map[string]interface{}{
		"testing/boolean": true,
	}

	assert.True(t, acc.HasMeasurement(req.Measurement))
	acc.AssertContainsTaggedFields(t, req.Measurement, fields, tags)

	// double
	req.MetricTypes = []string{"testing/double"}
	acc.GatherError(s.Gather)

	tags = map[string]string{
		"name":  "testing",
		"type":  "double",
		"alias": "float64",
	}
	fields = map[string]interface{}{
		"testing/double": 1.0,
	}

	assert.True(t, acc.HasMeasurement(req.Measurement))
	acc.AssertContainsTaggedFields(t, req.Measurement, fields, tags)

	// int64
	req.MetricTypes = []string{"testing/int64"}
	acc.GatherError(s.Gather)

	tags = map[string]string{
		"name":  "testing",
		"type":  "int64",
		"alias": "integer",
	}
	fields = map[string]interface{}{
		"testing/int64": int64(1),
	}

	assert.True(t, acc.HasMeasurement(req.Measurement))
	acc.AssertContainsTaggedFields(t, req.Measurement, fields, tags)

	// string
	req.Tags = &TimeSeriesTags{
		MetricLabels:   []string{"alias"},
		ResourceLabels: []string{"type"},
	}
	req.MetricTypes = []string{"testing/string"}
	acc.GatherError(s.Gather)

	tags = map[string]string{
		"type":  "string",
		"alias": "string",
	}
	fields = map[string]interface{}{
		"testing/string": "hello",
	}

	assert.True(t, acc.HasMeasurement(req.Measurement))
	acc.AssertContainsTaggedFields(t, req.Measurement, fields, tags)

	// distribution
	req.MetricTypes = []string{"testing/distribution"}
	acc.GatherError(s.Gather)

	tags = map[string]string{
		"type":  "distribution",
		"alias": "distribution",
	}
	fields = map[string]interface{}{
		"testing/distribution/Count":                 int64(1),
		"testing/distribution/Mean":                  1.0,
		"testing/distribution/SumOfSquaredDeviation": 2.0,
		"testing/distribution/Min":                   1.0,
		"testing/distribution/Max":                   2.0,
	}

	assert.True(t, acc.HasMeasurement(req.Measurement))
	acc.AssertContainsTaggedFields(t, req.Measurement, fields, tags)
}

func TestUpdateWindow(t *testing.T) {
	duration, _ := time.ParseDuration("1m")
	internalDuration := internal.Duration{
		Duration: duration,
	}

	c := &Stackdriver{
		Project:  "myproject",
		Delay:    internalDuration,
		Interval: internalDuration,
	}

	now := time.Now()

	assert.True(t, c.windowEnd.IsZero())
	assert.True(t, c.windowStart.IsZero())

	c.updateWindow(now)

	newStartTime := c.windowEnd

	// initial window just has a single period
	assert.EqualValues(t, c.windowEnd, now.Add(-c.Delay.Duration))
	assert.EqualValues(t, c.windowStart, now.Add(-c.Delay.Duration).Add(-c.Interval.Duration))

	now = time.Now()
	c.updateWindow(now)

	// subsequent window uses previous end time as start time
	assert.EqualValues(t, c.windowEnd, now.Add(-c.Delay.Duration))
	assert.EqualValues(t, c.windowStart, newStartTime)
}

func TestListTimeSeries(t *testing.T) {
	duration, _ := time.ParseDuration("1m")
	internalDuration := internal.Duration{
		Duration: duration,
	}
	req := &TimeSeriesRequest{
		Aggregation: &TimeSeriesAggregation{
			AlignmentPeriod:    internalDuration,
			PerSeriesAligner:   "ALIGN_MEAN",
			CrossSeriesReducer: "REDUCE_SUM",
			GroupByFields:      []string{"resource.label.target_proxy_name"},
		},
		Tags: &TimeSeriesTags{
			ResourceLabels: []string{"project_id", "target_proxy_name", "backend_name"},
		},
		Selectors: []*Selector{
			&Selector{
				Name:  "resource.label.target_proxy_name",
				Value: []string{"lb-ro-asia-proxy-target-proxy"},
			},
		},
	}
	s := &Stackdriver{
		Project:            "ro-mmo-ww",
		Interval:           internalDuration,
		Delay:              internalDuration,
		RateLimit:          200,
		TimeSeriesRequests: []*TimeSeriesRequest{req},
		client:             new(stackdriverMetricClient),
		pageSize:           10000,
	}
	if err := s.init(); err != nil {
		t.Fatal(err)
	}
	s.updateWindow(time.Now())

	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	_, err := s.client.ListTimeSeries(context.Background(), s.getListTimeSeriesRequest(req))
	const errText = `failed to create new metric client: google: could not find default credentials. See https://developers.google.com/accounts/docs/application-default-credentials for more information.`
	assert.Equal(t, errors.New(errText), err)

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credentials)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = s.client.ListTimeSeries(ctx, s.getListTimeSeriesRequest(req))
	assert.Equal(t, errors.New("failed to iterator time series: context deadline exceeded"), err)

	const vaildMetricType = "loadbalancing.googleapis.com/tcp_ssl_proxy/open_connections"
	req.MetricTypes = []string{vaildMetricType}
	timeSeriesReq := s.getListTimeSeriesRequest(req)
	timeSeriesReq.Filter = req.getMetricTypeFilterMap()[vaildMetricType]
	_, err = s.client.ListTimeSeries(context.Background(), timeSeriesReq)
	if err != nil {
		t.Error(err)
	}
}

func TestGetListTimeSeriesRequest(t *testing.T) {
	req := &TimeSeriesRequest{
		Aggregation: new(TimeSeriesAggregation),
	}
	s := &Stackdriver{
		Project: "myproject",
	}

	assert.Equal(t, int64(60), s.getListTimeSeriesRequest(req).Aggregation.AlignmentPeriod.Seconds)

	duration, _ := time.ParseDuration("2m")
	internalDuration := internal.Duration{
		Duration: duration,
	}
	req.Aggregation.AlignmentPeriod = internalDuration
	assert.Equal(t, int64(120), s.getListTimeSeriesRequest(req).Aggregation.AlignmentPeriod.Seconds)
}

func TestGetMetricTypeFilterMap(t *testing.T) {
	selectors := []*Selector{
		&Selector{
			Name:  "project",
			Value: []string{"p1"},
		},
		&Selector{
			Name:  "resource.labels.instance_id",
			Value: []string{`starts_with("x1")`, "x2"},
		},
	}
	timeSeries := &TimeSeriesRequest{
		MetricTypes: []string{"testing"},
		Selectors:   selectors,
	}
	expected := `metric.type = "testing" AND project = "p1" AND (resource.labels.instance_id = starts_with("x1") OR resource.labels.instance_id = "x2")`
	assert.Equal(t, expected, timeSeries.getMetricTypeFilterMap()["testing"])
}

func TestSampleConfig(t *testing.T) {
	assert.Equal(t, sampleConfig, new(Stackdriver).SampleConfig())
}

func TestDescription(t *testing.T) {
	assert.Equal(t, description, new(Stackdriver).Description())
}

func TestInitStackdriver(t *testing.T) {
	assert.Equal(t, errors.New("project is a required field for stackdriver input"), new(Stackdriver).init())

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	assert.Equal(t, errors.New("GOOGLE_APPLICATION_CREDENTIALS is not set"), (&Stackdriver{Project: "testing"}).init())

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "NONE")
	s := &Stackdriver{Project: "testing"}
	s.init()
	assert.Equal(t, int32(10000), s.pageSize)
	assert.ObjectsAreEqual(new(stackdriverMetricClient), s.client)
}
