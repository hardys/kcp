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

package main

import (
	"sync"
	"net/http"

	"golang.org/x/time/rate"
	"k8s.io/klog/v2"
	"k8s.io/apiserver/pkg/endpoints/request"
)

const (
	// Constant for the retry-after interval on rate limiting.
	// TODO: maybe make this dynamic? or user-adjustable?
	retryAfter = "1"

	requestLimit = 1
	burstLimit = 3

)


var limiters = make(map[string]*rate.Limiter)
var mu sync.Mutex

// Get a limiter for a given user if it exists,
// otherwise create one and store in the map
func getLimiter(user string) *rate.Limiter {
	mu.Lock()
    defer mu.Unlock()

	limiter, exists := limiters[user]
	if ! exists {
		limiter = rate.NewLimiter(requestLimit, burstLimit)
		limiters[user] = limiter
	}
	return limiter
}

// withRateLimitAuthenticatedUser limits the number of all requests
func withRateLimitAuthenticatedUser(handler http.Handler,) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, ok := request.UserFrom(req.Context())
		if !ok {
			klog.Errorf("can't detect user from context")
			return
		}
		// FIXME - seems like we should avoid using the name here but GetUID
		// returns empty, at least for kcp-admin - need to investigate
		limiter := getLimiter(user.GetName())
		if limiter.Allow() == false {
			if u, ok := request.UserFrom(req.Context()); ok {
				klog.Infof("SHDEBUG ratelimiting %s(%s)", u.GetUID(), u.GetName())
			}
			tooManyRequests(req, w)
			return
		}
		handler.ServeHTTP(w, req)
	})
}

// TODOLIST
// periodic cleanup of the limiters map
// ...


func tooManyRequests(req *http.Request, w http.ResponseWriter) {
	// Return a 429 status indicating "Too Many Requests"
	w.Header().Set("Retry-After", retryAfter)
	http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
}
