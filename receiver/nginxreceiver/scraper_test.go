// Copyright 2020, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nginxreceiver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"
)

func TestScraper(t *testing.T) {
	nginxMock := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/status" {
			rw.WriteHeader(200)
			_, _ = rw.Write([]byte(`Active connections: 291
server accepts handled requests
 16630948 16630948 31070465
Reading: 6 Writing: 179 Waiting: 106
`))
			return
		}
		rw.WriteHeader(404)
	}))
	sc := newNginxScraper(zap.NewNop(), &config{
		HTTPClientSettings: confighttp.HTTPClientSettings{
			Endpoint: nginxMock.URL + "/status",
		},
	})
	rms, err := sc.Scrape(context.Background())
	require.Nil(t, err)

	require.Equal(t, 1, rms.Len())
	rm := rms.At(0)

	ilms := rm.InstrumentationLibraryMetrics()
	require.Equal(t, 1, ilms.Len())

	ilm := ilms.At(0)
	ms := ilm.Metrics()

	require.Equal(t, 7, ms.Len())

	metricValues := make(map[string]int64, 7)

	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i)

		var dps pdata.IntDataPointSlice

		switch m.DataType() {
		case pdata.MetricDataTypeIntGauge:
			dps = m.IntGauge().DataPoints()
		case pdata.MetricDataTypeIntSum:
			dps = m.IntSum().DataPoints()
		}

		require.Equal(t, 1, dps.Len())

		metricValues[m.Name()] = dps.At(0).Value()
	}

	require.Equal(t, map[string]int64{
		"nginx.connections_accepted": 16630948,
		"nginx.connections_handled":  16630948,
		"nginx.requests":             31070465,
		"nginx.connections_active":   291,
		"nginx.connections_reading":  6,
		"nginx.connections_writing":  179,
		"nginx.connections_waiting":  106,
	}, metricValues)
}

func TestScraperError(t *testing.T) {
	nginxMock := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/status" {
			rw.WriteHeader(200)
			_, _ = rw.Write([]byte(`Bad status page`))
			return
		}
		rw.WriteHeader(404)
	}))
	t.Run("404", func(t *testing.T) {
		sc := newNginxScraper(zap.NewNop(), &config{
			HTTPClientSettings: confighttp.HTTPClientSettings{
				Endpoint: nginxMock.URL + "/badpath",
			},
		})

		_, err := sc.Scrape(context.Background())
		require.Equal(t, errors.New("expected 200 response, got 404"), err)
	})

	t.Run("parse error", func(t *testing.T) {
		sc := newNginxScraper(zap.NewNop(), &config{
			HTTPClientSettings: confighttp.HTTPClientSettings{
				Endpoint: nginxMock.URL + "/status",
			},
		})
		_, err := sc.Scrape(context.Background())
		require.Equal(t, errors.New("failed to parse response body \"Bad status page\": invalid input \"Bad status page\""), err)
	})
}
