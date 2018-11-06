# Google Cloud Platform Stackdriver Statistics Input

This plugin will pull Metric Statistics from Google Cloud Platform Stackdriver.

### Google Cloud Platform Authentication

This plugin uses a service account credential for Authentication with the Stackdriver
API endpoint. See: [Getting Started with Authentication](https://cloud.google.com/docs/authentication/getting-started)

### Configuration:

```toml
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

  ## Maximum requests per second, default value is 200.
  ratelimit = 200
  
  ## TimeSeries to Pull
  ## GCP Metric List: https://cloud.google.com/monitoring/api/metrics_gcp
  ## Agent Metric List: https://cloud.google.com/monitoring/api/metrics_agent
  ## AWS Metric List: https://cloud.google.com/monitoring/api/metrics_aws
  ## External Metrics List: https://cloud.google.com/monitoring/api/metrics_other
  [[inputs.stackdriver.timeseries]]
	# Measurement defaults to "stackdriver"
	# measurement = "stackdriver"

	## Specify at least one metric type
	## TimeSeries with any of the specific metric types will be pulled
	metric_types = [
	  "loadbalancing.googleapis.com/tcp_ssl_proxy/open_connections", 
	  "loadbalancing.googleapis.com/tcp_ssl_proxy/new_connections"
	]

	## The specific metric labels and resource labels are treated as tags (Optional)
	## Defaults to all metric labels and resource labels
	[inputs.stackdriver.timeseries.tags]
	  metric_labels = []
	  resource_labels = ["project_id", "target_proxy_name", "backend_name"]

    ## By default, the raw time series data is returned.
    ## Use this field to combine multiple time series for different views of the data.
    ## See: https://godoc.org/google.golang.org/genproto/googleapis/monitoring/v3#Aggregation.
    [inputs.stackdriver.timeseries.aggregation]
      alignment_period = "1m"
      per_series_aligner = "ALIGN_MEAN"
      cross_series_reducer = "REDUCE_NONE"
      groupby_fields = []
  
    ## Filter selectors for Metric. We consist of the logical AND of all selectors.
    ## See: https://cloud.google.com/monitoring/api/v3/filters.
    [[inputs.stackdriver.timeseries.selectors]]
      name = "resource.label.target_proxy_name"
      value = ["starts_with(\"lb-asia-proxy\")", "lb-europe-proxy"]
```

#### Restrictions and Limitations
- Stackdriver metrics are not available instantly via the Stackdriver API. You should adjust your collection `delay` to account for this lag in metrics availability based on your [monitoring latency](https://cloud.google.com/monitoring/api/v3/metrics-details#latency)
- Stackdriver API usage incurs cost - see [Stackdriver Monitoring Pricing](https://cloud.google.com/stackdriver/pricing#monitoring-costs)

### Measurements & Fields:

- Measurement: "stackdriver"
- Metric must be one of the following groups of metrics where metric types are predefined
  - [GCP Metric List](https://cloud.google.com/monitoring/api/metrics_gcp)
  - [Agent Metric List](https://cloud.google.com/monitoring/api/metrics_agent)
  - [AWS Metrics List](https://cloud.google.com/monitoring/api/metrics_aws)
  - [External Metrics List](https://cloud.google.com/monitoring/api/metrics_other)
- Aggregation describes how to combine multiple time series to provide different views of the data.
  - [Aggregation Definition](https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.alertPolicies#Aggregation)
- Selectors describes the use of monitoring filters to specify monitored resources, metric types, group definitions, and time series.
  - [Monitoring Filters](https://cloud.google.com/monitoring/api/v3/filters)

### Tags:
- By default we add all metric labels and monitored resource labels binding to the timeseries into tags
  - [GCP Metric Labels](https://cloud.google.com/monitoring/api/metrics_gcp)
  - [Agent Metric Labels](https://cloud.google.com/monitoring/api/metrics_agent)
  - [AWS Metrics Labels](https://cloud.google.com/monitoring/api/metrics_aws)
  - [External Metrics Labels](https://cloud.google.com/monitoring/api/metrics_other)
  - [Monitored Resource Labels](https://cloud.google.com/monitoring/api/resources)
- If `metric_labels` and `resource_labels` are set, then we add the specified labels into tags

### Example Output:

```
$ ./telegraf --config telegraf.conf --input-filter stackdriver --test
> stackdriver,project_id=myproject,target_proxy_name=lb-example1,backend_name=ig-example1,client_country=Sweden,proxy_continent=Europe loadbalancing.googleapis.com/tcp_ssl_proxy/open_connections=1000 1541178198000000000
> stackdriver,project_id=myproject,target_proxy_name=lb-example2,backend_name=ig-example2,client_country=Thailand,proxy_continent=Asia loadbalancing.googleapis.com/tcp_ssl_proxy/open_connections=2000 1541178198000000000
> stackdriver,project_id=myproject,target_proxy_name=lb-example1,backend_name=ig-example1,client_country=Sweden,proxy_continent=Europe  loadbalancing.googleapis.com/tcp_ssl_proxy/new_connections=50 1541178198000000000
> stackdriver,project_id=myproject,target_proxy_name=lb-example2,backend_name=ig-example2,client_country=Thailand,proxy_continent=Asia  loadbalancing.googleapis.com/tcp_ssl_proxy/new_connections=100 1541178198000000000
```
