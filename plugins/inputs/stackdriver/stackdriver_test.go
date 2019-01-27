package stackdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/monitoredres"

	"github.com/influxdata/telegraf/plugins/inputs"
	"github.com/influxdata/telegraf/testutil"
	"github.com/stretchr/testify/require"

	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

type mockGatherStackdriverClient struct{}

func (c *mockGatherStackdriverClient) ListMetricDescriptors(
	req *monitoringpb.ListMetricDescriptorsRequest,
) ([]*metricpb.MetricDescriptor, error) {
	switch req.Name {
	case "projects/listMetricDescriptorsError":
		return nil, errors.New("List MetricDescriptors Error")
	case "projects/listTimeSeriesError":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			new(metricpb.MetricDescriptor),
		}
		return metricDescriptorList, nil
	case "projects/distribution":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			&metricpb.MetricDescriptor{
				Type:      "testing/distribution",
				ValueType: metricpb.MetricDescriptor_DISTRIBUTION,
			},
		}
		return metricDescriptorList, nil
	case "projects/boolean":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			&metricpb.MetricDescriptor{
				Type:      "testing/boolean",
				ValueType: metricpb.MetricDescriptor_BOOL,
			},
		}
		return metricDescriptorList, nil
	case "projects/int64":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			&metricpb.MetricDescriptor{
				Type:      "testing/int64",
				ValueType: metricpb.MetricDescriptor_INT64,
			},
		}
		return metricDescriptorList, nil
	case "projects/double":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			&metricpb.MetricDescriptor{
				Type:      "testing/double",
				ValueType: metricpb.MetricDescriptor_DOUBLE,
			},
		}
		return metricDescriptorList, nil
	case "projects/string":
		metricDescriptorList := []*metricpb.MetricDescriptor{
			&metricpb.MetricDescriptor{
				Type:      "testing/string",
				ValueType: metricpb.MetricDescriptor_STRING,
			},
		}
		return metricDescriptorList, nil
	}
	return nil, nil
}

func (c *mockGatherStackdriverClient) ListTimeSeries(
	req *monitoringpb.ListTimeSeriesRequest,
) ([]*monitoringpb.TimeSeries, error) {
	point := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			EndTime: &timestamp.Timestamp{
				Seconds: time.Now().Unix(),
			},
		},
		Value: &monitoringpb.TypedValue{},
	}
	timeSeries := &monitoringpb.TimeSeries{
		Metric:   &metricpb.Metric{Labels: make(map[string]string)},
		Resource: &monitoredres.MonitoredResource{Labels: make(map[string]string)},
		Points:   []*monitoringpb.Point{point},
	}

	switch req.Name {
	case "projects/listTimeSeriesError":
		return nil, errors.New("List TimeSeries Error")
	case "projects/distribution":
		value := &monitoringpb.TypedValue_DistributionValue{
			DistributionValue: &distribution.Distribution{},
		}
		value.DistributionValue.Count = 1
		value.DistributionValue.Mean = 1.0
		value.DistributionValue.SumOfSquaredDeviation = 1.0
		value.DistributionValue.Range = &distribution.Distribution_Range{
			Min: 1.0,
			Max: 1.0,
		}
		value.DistributionValue.BucketCounts = []int64{1, 1}
		if strings.Contains(req.Filter, "LinearBuckets") {
			value.DistributionValue.BucketOptions = &distribution.Distribution_BucketOptions{
				Options: &distribution.Distribution_BucketOptions_LinearBuckets{
					LinearBuckets: &distribution.Distribution_BucketOptions_Linear{
						NumFiniteBuckets: 3,
						Width:            1,
						Offset:           1,
					},
				},
			}
		} else if strings.Contains(req.Filter, "ExponentialBuckets") {
			value.DistributionValue.BucketOptions = &distribution.Distribution_BucketOptions{
				Options: &distribution.Distribution_BucketOptions_ExponentialBuckets{
					ExponentialBuckets: &distribution.Distribution_BucketOptions_Exponential{
						NumFiniteBuckets: 3,
						GrowthFactor:     2,
						Scale:            1,
					},
				},
			}
		} else {
			value.DistributionValue.BucketOptions = &distribution.Distribution_BucketOptions{
				Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
					ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{
						Bounds: []float64{1.0, 2.0},
					},
				},
			}
		}
		point.Value.Value = value
		timeSeries.Metric.Labels["name"] = "projects/distribution"
		timeSeries.Resource.Type = "distribution"
		timeSeries.Resource.Labels["id"] = "0"
		timeSeries.ValueType = metricpb.MetricDescriptor_DISTRIBUTION
		return []*monitoringpb.TimeSeries{timeSeries}, nil
	case "projects/boolean":
		point.Value.Value = &monitoringpb.TypedValue_BoolValue{
			BoolValue: true,
		}
		timeSeries.Metric.Labels["name"] = "projects/boolean"
		timeSeries.Resource.Type = "boolean"
		timeSeries.Resource.Labels["id"] = "1"
		timeSeries.ValueType = metricpb.MetricDescriptor_BOOL
		return []*monitoringpb.TimeSeries{timeSeries}, nil
	case "projects/int64":
		point.Value.Value = &monitoringpb.TypedValue_Int64Value{
			Int64Value: 1,
		}
		timeSeries.Metric.Labels["name"] = "projects/int64"
		timeSeries.Resource.Type = "int64"
		timeSeries.Resource.Labels["id"] = "2"
		timeSeries.ValueType = metricpb.MetricDescriptor_INT64
		return []*monitoringpb.TimeSeries{timeSeries}, nil
	case "projects/double":
		point.Value.Value = &monitoringpb.TypedValue_DoubleValue{
			DoubleValue: 1.0,
		}
		timeSeries.Metric.Labels["name"] = "projects/double"
		timeSeries.Resource.Type = "double"
		timeSeries.Resource.Labels["id"] = "3"
		timeSeries.ValueType = metricpb.MetricDescriptor_DOUBLE
		return []*monitoringpb.TimeSeries{timeSeries}, nil
	case "projects/string":
		point.Value.Value = &monitoringpb.TypedValue_StringValue{
			StringValue: "1",
		}
		timeSeries.Metric.Labels["name"] = "projects/string"
		timeSeries.Resource.Type = "string"
		timeSeries.Resource.Labels["id"] = "4"
		timeSeries.ValueType = metricpb.MetricDescriptor_STRING
		return []*monitoringpb.TimeSeries{timeSeries}, nil
	}
	return nil, nil
}

func (c *mockGatherStackdriverClient) Close() error {
	return nil
}

func newGCPCredentials() (map[string]string, error) {
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return nil, errors.New("no credentials")
	}

	var credentials map[string]string
	data, err := ioutil.ReadFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &credentials); err != nil {
		return nil, err
	}

	return credentials, nil
}

func TestListMetricDescriptors(t *testing.T) {
	credentials, err := newGCPCredentials()
	if err != nil {
		return
	}

	s := &Stackdriver{
		RateLimit: 10,
	}
	if err = s.initializeStackdriverClient(); err != nil {
		return
	}
	defer s.client.Close()

	listMetricDescriptorsReq := &monitoringpb.ListMetricDescriptorsRequest{
		Name:   fmt.Sprintf("projects/%s", credentials["project_id"]),
		Filter: `metric.type = "loadbalancing.googleapis.com/https/request_count"`,
	}
	metricDescriptorList, err := s.client.ListMetricDescriptors(listMetricDescriptorsReq)
	if err != nil {
		return
	}

	require.NotZero(t, len(metricDescriptorList))
}

func TestListTimeSeries(t *testing.T) {
	credentials, err := newGCPCredentials()
	if err != nil {
		return
	}

	s := &Stackdriver{
		LookbackSeconds: 120,
		RateLimit:       10,
	}
	if err = s.initializeStackdriverClient(); err != nil {
		return
	}
	defer s.client.Close()

	listTimeSeriesReq := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", credentials["project_id"]),
		Filter: `metric.type = "loadbalancing.googleapis.com/https/request_count"`,
		Interval: &monitoringpb.TimeInterval{
			EndTime:   &timestamp.Timestamp{Seconds: time.Now().Add(-60 * time.Second).Unix()},
			StartTime: &timestamp.Timestamp{Seconds: time.Now().Unix() - s.LookbackSeconds},
		},
	}

	timeSeriesList, err := s.client.ListTimeSeries(listTimeSeriesReq)
	if err != nil {
		return
	}

	require.NotZero(t, len(timeSeriesList))
}

func TestInit(t *testing.T) {
	s := &Stackdriver{
		RateLimit:                       14,
		LookbackSeconds:                 120,
		DelaySeconds:                    60,
		ScrapeDistributionBuckets:       true,
		DistributionAggregationAligners: []string{},
	}
	require.Equal(t, s, inputs.Inputs["stackdriver"]())
}

func TestSampleConfig(t *testing.T) {
	require.Equal(t, sampleConfig, new(Stackdriver).SampleConfig())
}

func TestDescription(t *testing.T) {
	require.Equal(t, description, new(Stackdriver).Description())
}

func TestGather(t *testing.T) {
	var acc testutil.Accumulator
	s := &Stackdriver{ctx: context.Background()}

	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	require.Error(t, acc.GatherError(s.Gather))

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credentials)
	s.Project = "listMetricDescriptorsError"
	s.client = &mockGatherStackdriverClient{}
	require.EqualError(t, acc.GatherError(s.Gather), "List MetricDescriptors Error")
}

func TestGatherError(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "listTimeSeriesError",
		RateLimit: 10,
	}

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	}

	s.client = &mockGatherStackdriverClient{}
	require.EqualError(t, acc.GatherError(s.Gather), "List TimeSeries Error")
}

func TestGatherDistributionLinearBuckets(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "distribution",
		RateLimit: 10,
		Filter: &ListTimeSeriesFilter{
			ResourceLabels: []*Label{
				{Key: "name", Value: "LinearBuckets"},
			},
		},
		client: &mockGatherStackdriverClient{},
	}

	s.ScrapeDistributionBuckets = true
	fields := map[string]interface{}{
		"distribution_count":                    int64(1),
		"distribution_range_min":                1.0,
		"distribution_range_max":                1.0,
		"distribution_mean":                     1.0,
		"distribution_sum_of_squared_deviation": 1.0,
		"distribution_bucket_underflow":         int64(1),
		"distribution_bucket_ge_1.000":          int64(1),
		"distribution_bucket_ge_2.000":          0,
	}
	tags := map[string]string{
		"id":            "0",
		"name":          "projects/distribution",
		"resource_type": "distribution",
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
	acc.ClearMetrics()

	s.ScrapeDistributionBuckets = false
	s.DistributionAggregationAligners = []string{"ALIGN_PERCENTILE_UNKNOWN"}
	fields = map[string]interface{}{
		"distribution_align_none_count":                    int64(1),
		"distribution_align_none_range_min":                1.0,
		"distribution_align_none_range_max":                1.0,
		"distribution_align_none_mean":                     1.0,
		"distribution_align_none_sum_of_squared_deviation": 1.0,
		"distribution_align_none_bucket_underflow":         int64(1),
		"distribution_align_none_bucket_ge_1.000":          int64(1),
		"distribution_align_none_bucket_ge_2.000":          0,
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestGatherDistributionExponentialBuckets(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "distribution",
		RateLimit: 10,
		Filter: &ListTimeSeriesFilter{
			ResourceLabels: []*Label{
				{Key: "name", Value: "ExponentialBuckets"},
			},
		},
		client: &mockGatherStackdriverClient{},
	}

	s.ScrapeDistributionBuckets = true
	fields := map[string]interface{}{
		"distribution_count":                    int64(1),
		"distribution_range_min":                1.0,
		"distribution_range_max":                1.0,
		"distribution_mean":                     1.0,
		"distribution_sum_of_squared_deviation": 1.0,
		"distribution_bucket_underflow":         int64(1),
		"distribution_bucket_ge_1.000":          int64(1),
		"distribution_bucket_ge_2.000":          0,
	}
	tags := map[string]string{
		"id":            "0",
		"name":          "projects/distribution",
		"resource_type": "distribution",
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
	acc.ClearMetrics()

	s.ScrapeDistributionBuckets = false
	s.DistributionAggregationAligners = []string{"ALIGN_PERCENTILE_UNKNOWN"}
	fields = map[string]interface{}{
		"distribution_align_none_count":                    int64(1),
		"distribution_align_none_range_min":                1.0,
		"distribution_align_none_range_max":                1.0,
		"distribution_align_none_mean":                     1.0,
		"distribution_align_none_sum_of_squared_deviation": 1.0,
		"distribution_align_none_bucket_underflow":         int64(1),
		"distribution_align_none_bucket_ge_1.000":          int64(1),
		"distribution_align_none_bucket_ge_2.000":          0,
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}
func TestGatherDistributionExplicitBuckets(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "distribution",
		RateLimit: 10,
		Filter: &ListTimeSeriesFilter{
			ResourceLabels: []*Label{
				{Key: "name", Value: "ExplicitBuckets"},
			},
		},
		client: &mockGatherStackdriverClient{},
	}

	s.ScrapeDistributionBuckets = true
	fields := map[string]interface{}{
		"distribution_count":                    int64(1),
		"distribution_range_min":                1.0,
		"distribution_range_max":                1.0,
		"distribution_mean":                     1.0,
		"distribution_sum_of_squared_deviation": 1.0,
		"distribution_bucket_underflow":         int64(1),
		"distribution_bucket_ge_1.000":          int64(1),
		"distribution_bucket_ge_2.000":          0,
	}
	tags := map[string]string{
		"id":            "0",
		"name":          "projects/distribution",
		"resource_type": "distribution",
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
	acc.ClearMetrics()

	s.ScrapeDistributionBuckets = false
	s.DistributionAggregationAligners = []string{"ALIGN_PERCENTILE_999"}
	fields = map[string]interface{}{
		"distribution_align_none_count":                    int64(1),
		"distribution_align_none_range_min":                1.0,
		"distribution_align_none_range_max":                1.0,
		"distribution_align_none_mean":                     1.0,
		"distribution_align_none_sum_of_squared_deviation": 1.0,
		"distribution_align_none_bucket_underflow":         int64(1),
		"distribution_align_none_bucket_ge_1.000":          int64(1),
		"distribution_align_none_bucket_ge_2.000":          0,
	}
	require.NoError(t, acc.GatherError(s.Gather))
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestGatherBoolean(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "boolean",
		RateLimit: 10,
		client:    &mockGatherStackdriverClient{},
	}

	fields := map[string]interface{}{
		"boolean": true,
	}
	tags := map[string]string{
		"id":            "1",
		"name":          "projects/boolean",
		"resource_type": "boolean",
	}
	err := acc.GatherError(s.Gather)
	require.NoError(t, err)
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestGatherInt64(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "int64",
		RateLimit: 10,
		client:    &mockGatherStackdriverClient{},
	}

	fields := map[string]interface{}{
		"int64": int64(1),
	}
	tags := map[string]string{
		"id":            "2",
		"name":          "projects/int64",
		"resource_type": "int64",
	}
	err := acc.GatherError(s.Gather)
	require.NoError(t, err)
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestGatherDouble(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "double",
		RateLimit: 10,
		client:    &mockGatherStackdriverClient{},
	}

	fields := map[string]interface{}{
		"double": 1.0,
	}
	tags := map[string]string{
		"id":            "3",
		"name":          "projects/double",
		"resource_type": "double",
	}
	err := acc.GatherError(s.Gather)
	require.NoError(t, err)
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestGatherString(t *testing.T) {
	var acc testutil.Accumulator

	s := &Stackdriver{
		Project:   "string",
		RateLimit: 10,
		client:    &mockGatherStackdriverClient{},
	}

	fields := map[string]interface{}{
		"string": "1",
	}
	tags := map[string]string{
		"id":            "4",
		"name":          "projects/string",
		"resource_type": "string",
	}
	err := acc.GatherError(s.Gather)
	require.NoError(t, err)
	acc.AssertContainsTaggedFields(t, "testing", fields, tags)
}

func TestNewListTimeSeriesFilter(t *testing.T) {
	const metricType = "testing"
	s := &Stackdriver{}

	s.Filter = nil
	require.Equal(t, "", s.newListTimeSeriesFilter(""))

	s.Filter = &ListTimeSeriesFilter{
		ResourceLabels: []*Label{},
		MetricLabels:   []*Label{},
	}
	expected := fmt.Sprintf(`metric.type = "%s"`, metricType)

	s.Filter.ResourceLabels = append(s.Filter.ResourceLabels, &Label{
		Key:   "resource_key_1",
		Value: "resource_value_1",
	})
	expected1 := expected + fmt.Sprintf(` AND resource.labels.%s = "%s"`,
		s.Filter.ResourceLabels[0].Key, s.Filter.ResourceLabels[0].Value)
	require.Equal(t, expected1, s.newListTimeSeriesFilter(metricType))

	s.Filter.ResourceLabels = append(s.Filter.ResourceLabels, &Label{
		Key:   "resource_key_2",
		Value: `starts_with("resource")`,
	})
	expected2 := expected + fmt.Sprintf(` AND (resource.labels.%s = "%s" OR resource.labels.%s = %s)`,
		s.Filter.ResourceLabels[0].Key, s.Filter.ResourceLabels[0].Value,
		s.Filter.ResourceLabels[1].Key, s.Filter.ResourceLabels[1].Value)
	require.Equal(t, expected2, s.newListTimeSeriesFilter(metricType))

	s.Filter.MetricLabels = append(s.Filter.MetricLabels, &Label{
		Key:   "metric_key_1",
		Value: "metric_value_1",
	})
	expected3 := expected2 + fmt.Sprintf(` AND metric.labels.%s = "%s"`,
		s.Filter.MetricLabels[0].Key, s.Filter.MetricLabels[0].Value)
	require.Equal(t, expected3, s.newListTimeSeriesFilter(metricType))

	s.Filter.MetricLabels = append(s.Filter.MetricLabels, &Label{
		Key:   "metric_key_2",
		Value: `starts_with("metric")`,
	})
	expected4 := expected2 + fmt.Sprintf(` AND (metric.labels.%s = "%s" OR metric.labels.%s = %s)`,
		s.Filter.MetricLabels[0].Key, s.Filter.MetricLabels[0].Value,
		s.Filter.MetricLabels[1].Key, s.Filter.MetricLabels[1].Value)
	require.Equal(t, expected4, s.newListTimeSeriesFilter(metricType))
}

func TestNewTimeSeriesConf(t *testing.T) {
	metricType := "testing"

	s := &Stackdriver{
		Project:         "myproject",
		LookbackSeconds: 600,
		Filter: &ListTimeSeriesFilter{
			ResourceLabels: []*Label{
				{Key: "r1", Value: "v1"},
				{Key: "r2", Value: "starts_with(v2)"},
			},
			MetricLabels: []*Label{
				{Key: "m1", Value: "v1"},
				{Key: "m2", Value: "ends_with(v2)"},
			},
		},
	}
	cfg := s.newTimeSeriesConf(metricType)
	require.Equal(t, metricType, cfg.measurement)

	metricType = "package/testing"
	cfg = s.newTimeSeriesConf(metricType)
	require.Equal(t, strings.Split(metricType, "/")[0], cfg.measurement)
}

func TestInitForDistribution(t *testing.T) {
	const fieldPrefix = "testing"

	tsConf := &timeSeriesConf{
		fieldPrefix: fieldPrefix,
	}
	tsConf.initForDistribution()
	require.Equal(t, fieldPrefix+"_", tsConf.fieldPrefix)
}

func TestTimeSeriesConfCacheIsValid(t *testing.T) {
	c := &TimeSeriesConfCache{
		TTL: 60 * time.Second,
	}
	require.False(t, c.IsValid())

	c.TimeSeriesConfs = []*timeSeriesConf{
		{
			fieldPrefix: "value",
		},
	}
	c.Generated = time.Now().Add(-61 * time.Second)
	require.False(t, c.IsValid())

	c.Generated = time.Now().Add(-59 * time.Second)
	require.True(t, c.IsValid())
}

func TestInitializeStackdriverClient(t *testing.T) {
	s := &Stackdriver{}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		require.NoError(t, s.initializeStackdriverClient())
	}

	s.client = &mockGatherStackdriverClient{}
	require.NoError(t, s.initializeStackdriverClient())
}

func TestIncludeMetricType(t *testing.T) {
	s := &Stackdriver{}
	require.True(t, s.includeMetricType("NotSpecifiedMetricType"))

	s.ExcludeMetricTypePrefixes = []string{
		"excludeMetricType1",
		"excludeMetricType2",
	}
	for _, excludeStr := range s.ExcludeMetricTypePrefixes {
		require.False(t, s.includeMetricType(excludeStr))
	}
	require.True(t, s.includeMetricType("NotSpecifiedMetricType"))
}

func TestIncludeTag(t *testing.T) {
	s := &Stackdriver{}
	require.True(t, s.includeTag("NotSpecifiedTag"))

	s.ExcludeTagPrefixes = []string{
		"excludeTag1",
		"excludeTag2",
	}
	for _, excludeStr := range s.ExcludeMetricTypePrefixes {
		require.False(t, s.includeTag(excludeStr))
	}
	require.True(t, s.includeTag("NotSpecifiedTag"))
}

func TestGeneratetimeSeriesConfs(t *testing.T) {
	s := &Stackdriver{
		client: &mockGatherStackdriverClient{},
	}

	s.timeSeriesConfCache = &TimeSeriesConfCache{
		TTL:       60 * time.Second,
		Generated: time.Now(),
		TimeSeriesConfs: []*timeSeriesConf{
			{
				measurement: "test",
				fieldPrefix: "value",
				listTimeSeriesRequest: &monitoringpb.ListTimeSeriesRequest{
					Interval: &monitoringpb.TimeInterval{},
				},
			},
		},
	}
	tsConf, err := s.generatetimeSeriesConfs()
	require.NoError(t, err)
	require.Equal(t, s.timeSeriesConfCache.TimeSeriesConfs, tsConf)

	s.timeSeriesConfCache = nil
	s.Project = "listMetricDescriptorsError"
	tsConf, err = s.generatetimeSeriesConfs()
	require.Empty(t, tsConf)
	require.Error(t, err)

	s.Project = "distribution"
	s.ScrapeDistributionBuckets = true
	s.IncludeMetricTypePrefixes = []string{"testing"}
	s.DistributionAggregationAligners = []string{
		"ALIGN_PERCENTILE_99",
		"ALIGN_PERCENTILE_95",
		"ALIGN_PERCENTILE_50",
	}
	tsConf, err = s.generatetimeSeriesConfs()
	require.NotEmpty(t, tsConf)
	require.NoError(t, err)

	s.Project = "boolean"
	s.IncludeMetricTypePrefixes = nil
	tsConf, err = s.generatetimeSeriesConfs()
	require.NotEmpty(t, tsConf)
	require.NoError(t, err)

	s.ExcludeMetricTypePrefixes = []string{"testing/boolean"}
	tsConf, err = s.generatetimeSeriesConfs()
	require.Empty(t, tsConf)
	require.NoError(t, err)
}
