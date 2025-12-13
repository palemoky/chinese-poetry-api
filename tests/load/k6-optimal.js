import http from 'k6/http';
import { check, sleep } from 'k6';

// Optimal load test - find the sweet spot
export const options = {
    stages: [
        { duration: '30s', target: 200 },  // Warm up
        { duration: '1m', target: 400 },   // Ramp to 400
        { duration: '2m', target: 400 },   // Hold at 400
        { duration: '1m', target: 600 },   // Ramp to 600
        { duration: '2m', target: 600 },   // Hold at 600
        { duration: '1m', target: 800 },   // Ramp to 800
        { duration: '3m', target: 800 },   // Hold at 800 (sweet spot)
        { duration: '30s', target: 0 },    // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<1000', 'p(99)<2000'],
        http_req_failed: ['rate<0.01'],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:1279';

export default function () {
    const endpoints = [
        '/api/v1/poems?page=1&page_size=20',
        '/api/v1/poems/random',
        '/api/v1/poems/random?author=李白',
        '/api/v1/poems/search?q=月',
    ];

    const endpoint = endpoints[Math.floor(Math.random() * endpoints.length)];
    const res = http.get(`${BASE_URL}${endpoint}`);

    check(res, {
        'status is 200': (r) => r.status === 200,
        'response time < 3s': (r) => r.timings.duration < 3000,
    });

    sleep(1);
}
