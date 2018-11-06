package stackdriver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	gax "github.com/googleapis/gax-go"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/internal"
	"github.com/influxdata/telegraf/internal/limiter"
	"github.com/influxdata/telegraf/plugins/inputs"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

const (
	description  = "Pull TimeSeries Statistics from Google Stackdriver"
	sampleConfig = `
  # GCP Project ID
  project = "myproject"
  
  # The minimum period for Stackdriver timeseries is 1 minute (60s). However not all
  # timeseries are made available to the 1 minute period. Some are collected at
  # 3 minute, 5 minute, or larger intervals.
  # Note that if a period is configured that is smaller than the minimum for a
  # particular timeseries, that timeseries will not be returned by the Stackdriver API
  # and will not be collected by Telegraf.
  #
  # Requested Stackdriver aggregation period (required - must be a multiple of 60s)
  interval = "5m"
  
  ## Collection Delay (required - must account for metrics availability via Stackdriver API)
  delay = "1m"

  ## Configure the TTL for the internal cache of metrics.
  ## Defaults to 1 hr if not specified
  #cache_ttl = "10m"

  ## Maximum requests per second, default value is 200.
  ratelimit = 200
  
  ## TimeSeries to Pull
  ## GCP Metric List: https://cloud.google.com/monitoring/api/metrics_gcp
  ## Agent Metric List: https://cloud.google.com/monitoring/api/metrics_agent
  ## AWS Metric List: https://cloud.google.com/monitoring/api/metrics_aws
  ## External Metrics List: https://cloud.google.com/monitoring/api/metrics_other
  [[inputs.stackdriver.timeseries]]
	## Specify metric type prefix which must endswith "/" (required)
	## TimeSeries with the specified metric type prefix will be pulled
	metric_type_prefix = "loadbalancing.googleapis.com/tcp_ssl_proxy/"

	## The specified metric labels and resource labels are treated as tags (optional)
	## Defaults to all metric labels and resource labels
	[inputs.stackdriver.timeseries.tags]
	  metric_labels = []
	  resource_labels = ["project_id", "target_proxy_name", "backend_name"]

    ## By default, the raw time series data is returned.
    ## Use this field to combine multiple time series for different views of the data.
    ## See: https://godoc.org/google.golang.org/genproto/googleapis/monitoring/v3#Aggregation.
    [inputs.stackdriver.timeseries.aggregation]
      alignment_period = "1m"
      aligner = "ALIGN_NONE"

    ## Filter selectors for Metric. We consist of the logical AND of all selectors.
    ## See: https://cloud.google.com/monitoring/api/v3/filters.
    [[inputs.stackdriver.timeseries.filters]]
      name = "resource.label.target_proxy_name"
      value = ['starts_with("lb-asia-proxy")', "lb-europe-proxy"]
`
)

type (
	// Stackdriver is the Google Stackdriver config info.
	Stackdriver struct {
		Project            string               `toml:"project"`
		Interval           internal.Duration    `toml:"interval"`
		Delay              internal.Duration    `toml:"delay"`
		CacheTTL           internal.Duration    `toml:"cache_ttl"`
		RateLimit          int                  `toml:"ratelimit"`
		TimeSeriesRequests []*TimeSeriesRequest `toml:"timeseries"`

		client      stackdriverClient
		metricCache *MetricTypeCache
		windowStart time.Time
		windowEnd   time.Time
	}

	// TimeSeriesRequest describes how to get timeseries from stackdriver API.
	TimeSeriesRequest struct {
		MetricTypePrefix string                 `toml:"metric_type_prefix"`
		Tags             *TimeSeriesTags        `toml:"tags"`
		Aggregation      *TimeSeriesAggregation `toml:"aggregation"`
		Filters          []*Filter              `toml:"filters"`
	}

	// TimeSeriesTags describes how to set timeseries tags
	TimeSeriesTags struct {
		MetricLabels   []string `toml:"metric_labels"`
		ResourceLabels []string `toml:"resource_labels"`
	}

	// TimeSeriesAggregation describes how to combine
	// multiple time series to provide different views of
	// the data.
	TimeSeriesAggregation struct {
		AlignmentPeriod internal.Duration `toml:"alignment_period"`
		Aligner         string            `toml:"aligner"`
	}

	// Filter consists of metric filter.
	Filter struct {
		Name  string   `toml:"name"`
		Value []string `toml:"value"`
	}

	// MetricTypeCache keeps metric types in memory for a period
	MetricTypeCache struct {
		TTL         time.Duration
		Fetched     time.Time
		MetricTypes map[string][]string
	}

	stackdriverMetricClient struct{}

	stackdriverClient interface {
		ListTimeSeries(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest, opts ...gax.CallOption) ([]*monitoringpb.TimeSeries, error)
	}
)

func init() {
	inputs.Add("stackdriver", func() telegraf.Input {
		return &Stackdriver{
			RateLimit: 200,
		}
	})
}

// ListTimeSeries implements stackdriverClient interface
func (c *stackdriverMetricClient) ListTimeSeries(
	ctx context.Context,
	req *monitoringpb.ListTimeSeriesRequest,
	opts ...gax.CallOption,
) ([]*monitoringpb.TimeSeries, error) {
	var timeSeriesList []*monitoringpb.TimeSeries

	metricClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric client: %v", err)
	}
	defer metricClient.Close() // nolint: errcheck

	iter := metricClient.ListTimeSeries(ctx, req)
	for {
		timeSeries, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, fmt.Errorf("failed to iterator time series: %v", err)
		}
		timeSeriesList = append(timeSeriesList, timeSeries)
	}

	return timeSeriesList, nil
}

// SampleConfig implements telegraf.Input interface
func (s *Stackdriver) SampleConfig() string {
	return sampleConfig
}

// Description implements telegraf.Input interface
func (s *Stackdriver) Description() string {
	return description
}

// Gather implements telegraf.Input interface
func (s *Stackdriver) Gather(acc telegraf.Accumulator) error {
	if s.client == nil {
		if err := s.init(); err != nil {
			return err
		}
	}

	s.updateWindow(time.Now())

	// limit concurrency or we can easily exhaust user connection limit
	lmtr := limiter.NewRateLimiter(s.RateLimit, time.Second)
	defer lmtr.Stop()

	var wg sync.WaitGroup
	wg.Add(len(s.TimeSeriesRequests))

	for _, request := range s.TimeSeriesRequests {
		<-lmtr.C
		go func(req *TimeSeriesRequest) {
			defer wg.Done()
			acc.AddError(s.gatherTimeSeries(acc, req))
		}(request)
	}
	wg.Wait()

	return nil
}

func (s *Stackdriver) init() error {
	if s.Project == "" {
		return errors.New("project is a required field for stackdriver input")
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return errors.New("GOOGLE_APPLICATION_CREDENTIALS is not set")
	}
	for _, timeSeriesRequest := range s.TimeSeriesRequests {
		if !strings.HasSuffix(timeSeriesRequest.MetricTypePrefix, "/") {
			return fmt.Errorf(`metric type prefix "%s" does not endswith "/"`, timeSeriesRequest.MetricTypePrefix)
		}
	}

	s.client = new(stackdriverMetricClient)

	return nil
}

func (s *Stackdriver) updateWindow(relativeTo time.Time) {
	windowsEnd := relativeTo.Add(-s.Delay.Duration)

	if s.windowEnd.IsZero() {
		s.windowStart = windowsEnd.Add(-s.Interval.Duration)
	} else {
		s.windowStart = s.windowEnd
	}

	s.windowEnd = windowsEnd
}

func (s *Stackdriver) gatherTimeSeries(acc telegraf.Accumulator, req *TimeSeriesRequest) error {
	if req.MetricTypePrefix == "" {
		return errors.New("metric_type_prefixes is a required field for stackdriver input")
	}

	timeSeriesMap, err := s.getTimeSeriesMap(req)
	if err != nil {
		return err
	}

	for metricType, timeSeries := range timeSeriesMap {
		measurement := metricType
		fieldName := "value"
		if slashIdx := strings.LastIndex(metricType, "/"); slashIdx > 0 {
			measurement = metricType[:slashIdx]
			fieldName = metricType[slashIdx+1:]
		}
		if req.Aggregation.Aligner == "" {
			req.Aggregation.Aligner = "ALIGN_NONE"
		}
		aligner := snakeCase(req.Aggregation.Aligner)
		fieldName = fmt.Sprintf("%s_%s", fieldName, aligner)

		for _, timeSeriesData := range timeSeries {
			metricLabels := timeSeriesData.GetMetric().GetLabels()
			resourceLabels := timeSeriesData.GetResource().GetLabels()
			tags := make(map[string]string)

			// add metric labels into tags
			if req.Tags != nil && len(req.Tags.MetricLabels) > 0 {
				for _, label := range req.Tags.MetricLabels {
					if value, ok := metricLabels[label]; ok {
						tags[label] = value
					}
				}
			} else {
				for label, value := range metricLabels {
					tags[label] = value
				}
			}
			// add resource labels into tags
			if req.Tags != nil && len(req.Tags.ResourceLabels) > 0 {
				for _, label := range req.Tags.ResourceLabels {
					if value, ok := resourceLabels[label]; ok {
						tags[label] = value
					}
				}
			} else {
				for label, value := range resourceLabels {
					tags[label] = value
				}
			}

			fields := make(map[string]interface{})
			for _, point := range timeSeriesData.GetPoints() {
				switch timeSeriesData.GetValueType() { // value type: bool, double, int64, string, distribution
				case metric.MetricDescriptor_BOOL:
					fields[fieldName+"_"+aligner] = point.GetValue().GetBoolValue()
				case metric.MetricDescriptor_DOUBLE:
					fields[fieldName+"_"+aligner] = point.GetValue().GetDoubleValue()
				case metric.MetricDescriptor_INT64:
					fields[fieldName+"_"+aligner] = point.GetValue().GetInt64Value()
				case metric.MetricDescriptor_STRING:
					fields[fieldName+"_"+aligner] = point.GetValue().GetStringValue()
				case metric.MetricDescriptor_DISTRIBUTION:
					fields[fieldName+"_"+"count"] = point.GetValue().GetDistributionValue().GetCount()
					fields[fieldName+"_"+"min"] = point.GetValue().GetDistributionValue().GetRange().GetMin()
					fields[fieldName+"_"+"max"] = point.GetValue().GetDistributionValue().GetRange().GetMax()
					fields[fieldName+"_"+"mean"] = point.GetValue().GetDistributionValue().GetMean()
					fields[fieldName+"_"+"sum_of_square_deviation"] = point.GetValue().GetDistributionValue().GetSumOfSquaredDeviation()
				}
				acc.AddFields(measurement, fields, tags, s.windowEnd)
			}
		}
	}

	return nil
}

func (s *Stackdriver) getTimeSeriesMap(req *TimeSeriesRequest) (map[string][]*monitoringpb.TimeSeries, error) {
	metricTypeFilters, err := s.getMetricTypeFilters(req)
	if err != nil {
		return nil, err
	}

	timeSeriesMap := make(map[string][]*monitoringpb.TimeSeries)
	for metricType, filterString := range metricTypeFilters {
		listTimeSeriesRequest := s.getListTimeSeriesRequest(req)
		listTimeSeriesRequest.Filter = filterString
		timeSeriesList, err := s.client.ListTimeSeries(context.Background(), listTimeSeriesRequest)
		if err != nil {
			return nil, err
		}
		timeSeriesMap[metricType] = timeSeriesList
	}

	return timeSeriesMap, nil
}

func (s *Stackdriver) getListTimeSeriesRequest(req *TimeSeriesRequest) *monitoringpb.ListTimeSeriesRequest {
	if req.Aggregation == nil {
		req.Aggregation = &TimeSeriesAggregation{
			AlignmentPeriod: internal.Duration{Duration: 60 * time.Second},
			Aligner:         "ALIGN_NONE",
		}
	} else if int64(req.Aggregation.AlignmentPeriod.Duration.Seconds()) == 0 {
		req.Aggregation.AlignmentPeriod = internal.Duration{Duration: 60 * time.Second}
	}

	return &monitoringpb.ListTimeSeriesRequest{
		Name: fmt.Sprintf("projects/%s", s.Project),
		Interval: &monitoringpb.TimeInterval{
			EndTime:   &timestamp.Timestamp{Seconds: s.windowEnd.Unix()},
			StartTime: &timestamp.Timestamp{Seconds: s.windowStart.Unix()},
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod: &duration.Duration{
				Seconds: int64(req.Aggregation.AlignmentPeriod.Duration.Seconds()),
			},
			PerSeriesAligner:   monitoringpb.Aggregation_Aligner(monitoringpb.Aggregation_Aligner_value[req.Aggregation.Aligner]),
			CrossSeriesReducer: monitoringpb.Aggregation_Reducer(monitoringpb.Aggregation_Reducer_value["REDUCE_NONE"]),
		},
		PageSize: 10000,
	}
}

func (s *Stackdriver) getMetricTypeFilters(req *TimeSeriesRequest) (map[string]string, error) {
	metricTypes, err := s.fetchMetricTypes()
	if err != nil {
		return nil, err
	}

	metricTypeNames, ok := metricTypes[req.MetricTypePrefix]
	if !ok {
		return nil, fmt.Errorf("no metric type prefixes with %s", req.MetricTypePrefix)
	}

	filterMap := make(map[string]string)
	functions := []string{
		"starts_with",
		"ends_with",
		"has_substring",
		"one_of",
	}

	for _, metricTypeName := range metricTypeNames {
		filterString := fmt.Sprintf(`metric.type = "%s"`, metricTypeName)
		for _, filter := range req.Filters {
			if len(filter.Value) > 0 {
				values := make([]string, len(filter.Value))
				for i, v := range filter.Value {
					if hasPrefix(v, functions...) {
						values[i] = fmt.Sprintf(`%s = %s`, filter.Name, v)
					} else {
						values[i] = fmt.Sprintf(`%s = "%s"`, filter.Name, v)
					}
				}
				if len(values) == 1 {
					filterString += fmt.Sprintf(" AND %s", strings.Join(values, " OR "))
				} else {
					filterString += fmt.Sprintf(" AND (%s)", strings.Join(values, " OR "))
				}
			}
		}
		filterMap[metricTypeName] = filterString
	}
	return filterMap, nil
}

func (s *Stackdriver) fetchMetricTypes() (map[string][]string, error) {
	if s.metricCache != nil && s.metricCache.IsValid() {
		return s.metricCache.MetricTypes, nil
	}

	metricClient, err := monitoring.NewMetricClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create metric client: %v", err)
	}
	defer metricClient.Close() // nolint: errcheck

	listMetricDescriptorsRequest := &monitoringpb.ListMetricDescriptorsRequest{
		Name: fmt.Sprintf("projects/%s", s.Project),
	}

	metricTypes := make(map[string][]string) // simulate a hash set to avoid duplicate metric type name
	for _, timeSeriesReq := range s.TimeSeriesRequests {
		metricTypePrefix := timeSeriesReq.MetricTypePrefix
		listMetricDescriptorsRequest.Filter = fmt.Sprintf(`starts_with("%s")`, metricTypePrefix)
		iter := metricClient.ListMetricDescriptors(context.Background(), listMetricDescriptorsRequest)
		for {
			metricDescriptors, err := iter.Next()
			if err != nil {
				if err == iterator.Done {
					break
				}
				return nil, fmt.Errorf("failed to iterator metric descriptors: %v", err)
			}
			metricTypes[metricTypePrefix] = append(metricTypes[metricTypePrefix], metricDescriptors.Name)
		}
	}

	s.metricCache = &MetricTypeCache{
		TTL:         s.CacheTTL.Duration,
		Fetched:     time.Now(),
		MetricTypes: metricTypes,
	}

	return metricTypes, nil
}

// IsValid checks metric cache validity
func (c *MetricTypeCache) IsValid() bool {
	return c.MetricTypes != nil && time.Since(c.Fetched) < c.TTL
}

func hasPrefix(s string, prefix ...string) bool {
	for _, v := range prefix {
		if strings.HasPrefix(s, v) {
			return true
		}
	}
	return false
}

// Formatting helpers
func formatField(metricName string, statistic string) string {
	return fmt.Sprintf("%s_%s", snakeCase(metricName), snakeCase(statistic))
}

func formatMeasurement(namespace string) string {
	namespace = strings.Replace(namespace, "/", "_", -1)
	namespace = snakeCase(namespace)
	return fmt.Sprintf("cloudwatch_%s", namespace)
}

func snakeCase(s string) string {
	s = internal.SnakeCase(s)
	s = strings.Replace(s, "__", "_", -1)
	return s
}
