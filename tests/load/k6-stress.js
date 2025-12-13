import http from 'k6/http';
import { check, sleep } from 'k6';

// Stress test configuration - optimized based on performance testing
export const options = {
    stages: [
        { duration: '1m', target: 500 },   // Ramp up to 500 users
        { duration: '2m', target: 800 },   // Ramp up to 800 users
        { duration: '3m', target: 800 },   // Stay at 800 users (sweet spot)
        { duration: '1m', target: 1200 },  // Push to 1200 users
        { duration: '2m', target: 1200 },  // Stay at 1200 users (observe degradation)
        { duration: '1m', target: 0 },     // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<3000', 'p(99)<5000'], // Realistic for high load
        http_req_failed: ['rate<0.05'],                    // Allow 5% error rate
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
