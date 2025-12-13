import http from 'k6/http';
import { check, sleep } from 'k6';

// Stress test configuration - push to the limit
export const options = {
    stages: [
        { duration: '1m', target: 500 },   // Ramp up to 500 users
        { duration: '3m', target: 1000 },  // Ramp up to 1000 users
        { duration: '5m', target: 1000 },  // Stay at 1000 users
        { duration: '1m', target: 2000 },  // Spike to 2000 users
        { duration: '2m', target: 2000 },  // Stay at 2000 users
        { duration: '1m', target: 0 },     // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<1000', 'p(99)<2000'], // More relaxed for stress test
        http_req_failed: ['rate<0.05'],                   // Allow 5% error rate
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:1279';

export default function () {
    // Focus on most demanding endpoints
    const endpoints = [
        '/api/v1/poems?page=1&page_size=50',
        '/api/v1/poems/random',
        '/api/v1/poems/random?author=李白&type=五言绝句',
        '/api/v1/poems/search?q=月',
        '/api/v1/authors?page=1&page_size=50',
    ];

    const endpoint = endpoints[Math.floor(Math.random() * endpoints.length)];
    const res = http.get(`${BASE_URL}${endpoint}`);

    check(res, {
        'status is 200': (r) => r.status === 200,
    });

    sleep(0.5); // Shorter sleep for stress test
}
