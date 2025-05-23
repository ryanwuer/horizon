// Copyright © 2023 Horizoncd.
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

package metrics

import (
	"fmt"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/horizoncd/horizon/core/common"
	middleware "github.com/horizoncd/horizon/core/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	_handlerLabel = "handler"
	_verbLabel    = "verb"
	_codeLabel    = "code"
	_callerLabel  = "caller"
)

var (
	apiHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "horizon_request_duration_seconds",
			Help: "horizon request duration seconds histogram.",
			Buckets: []float64{0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60},
		},
		[]string{_handlerLabel, _verbLabel, _callerLabel},
	)

	apiCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "horizon_request_total",
			Help: "horizon request total counter.",
		},
		[]string{_handlerLabel, _verbLabel, _codeLabel, _callerLabel},
	)
)

// Middleware report metrics of handler
func Middleware(skippers ...middleware.Skipper) gin.HandlerFunc {
	return middleware.New(func(c *gin.Context) {
		// start timer
		start := time.Now()

		c.Next()

		// end timer
		latency := time.Since(start)

		var handler string
		if handler = func() string {
			handlerName := c.HandlerName()
			if regexp.MustCompile(`.*func\d*$`).MatchString(handlerName) {
				return ""
			}
			return handlerName
		}(); handler == "" {
			return
		}

		statusCode := c.Writer.Status()
		method := c.Request.Method
		caller := common.CallerFromContext(c)

		apiHistogram.With(prometheus.Labels{
			_handlerLabel: handler,
			_verbLabel:    method,
			_callerLabel:  caller,
		}).Observe(latency.Seconds())

		apiCounter.With(prometheus.Labels{
			_handlerLabel: handler,
			_verbLabel:    method,
			_codeLabel:    fmt.Sprintf("%v", statusCode),
			_callerLabel:  caller,
		}).Inc()
	}, skippers...)
}
