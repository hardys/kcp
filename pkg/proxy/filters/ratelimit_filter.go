/*
Copyright 2022 The KCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filters

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"
)

const (
	// Constant for the retry-after interval on rate limiting.
	// TODO: maybe make this dynamic? or user-adjustable?
	retryAfter = "1"

	requestLimit = 1
	burstLimit   = 3
)

type userLimiter struct {
	limiter *rate.Limiter
	lastSeen time.Time
}

var limiters = make(map[string]*userLimiter)
var mu sync.Mutex

// Get a limiter for a given user if it exists,
// otherwise create one and store in the map
func getLimiter(user string) (*userLimiter, int) {
	mu.Lock()
	defer mu.Unlock()

	limiter, exists := limiters[user]
	if !exists {
		newLimiter := rate.NewLimiter(requestLimit, burstLimit)
		limiter = &userLimiter{newLimiter, time.Now()}
		limiters[user] = limiter
	}
	limiter.lastSeen = time.Now()
	return limiter, len(limiters)
}

func CleanupLimiters() {
	for {
		time.Sleep(time.Minute)

		mu.Lock()
		for user, limiter := range limiters {
			if time.Since(limiter.lastSeen) > 1*time.Minute {
				klog.Infof("SHDEBUG deleting limiter for %v", user)
				delete(limiters, user)
			}
		}
		mu.Unlock()
	}
}

// WithRateLimitAuthenticatedUser limits the number of all requests
func WithRateLimitAuthenticatedUser(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, ok := request.UserFrom(req.Context())
		if !ok {
			klog.Errorf("can't detect user from context")
			return
		}
		// FIXME - seems like we should avoid using the name here but GetUID
		// returns empty, at least for kcp-admin - need to investigate
		limiter, num := getLimiter(user.GetName())
		if limiter.limiter.Allow() == false {
			if u, ok := request.UserFrom(req.Context()); ok {
				klog.Infof("SHDEBUG ratelimiting %s(%s) len=%d", u.GetUID(), u.GetName(), num)
			}
			tooManyRequests(req, w)
			return
		}
		handler.ServeHTTP(w, req)
	})
}

func tooManyRequests(req *http.Request, w http.ResponseWriter) {
	// Return a 429 status indicating "Too Many Requests"
	w.Header().Set("Retry-After", retryAfter)
	http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
}
