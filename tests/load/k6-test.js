import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');

// Test configuration
export const options = {
    stages: [
        { duration: '30s', target: 50 },   // Ramp up to 50 users
        { duration: '1m', target: 100 },   // Ramp up to 100 users
        { duration: '2m', target: 100 },   // Stay at 100 users
        { duration: '30s', target: 200 },  // Spike to 200 users
        { duration: '1m', target: 200 },   // Stay at 200 users
        { duration: '30s', target: 0 },    // Ramp down to 0 users
    ],
    thresholds: {
        http_req_duration: ['p(95)<2500', 'p(99)<3000'], // 95% < 2.5s, 99% < 3s
        http_req_failed: ['rate<0.01'],                  // Error rate < 1%
        errors: ['rate<0.05'],                           // Custom error rate < 5%
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:1279';

// Test scenarios
export default function () {
    // Scenario 1: List poems (most common)
    const listRes = http.get(`${BASE_URL}/api/v1/poems?page=1&page_size=20`);
    check(listRes, {
        'list poems status 200': (r) => r.status === 200,
        'list poems has data': (r) => JSON.parse(r.body).data.length > 0,
    }) || errorRate.add(1);

    sleep(1);

    // Scenario 2: Random poem (popular feature)
    const randomRes = http.get(`${BASE_URL}/api/v1/poems/random`);
    check(randomRes, {
        'random poem status 200': (r) => r.status === 200,
        'random poem has title': (r) => JSON.parse(r.body).title !== undefined,
    }) || errorRate.add(1);

    sleep(1);

    // Scenario 3: Random poem with filter
    const filteredRandomRes = http.get(`${BASE_URL}/api/v1/poems/random?author=李白`);
    check(filteredRandomRes, {
        'filtered random status 200': (r) => r.status === 200,
        'filtered random has author': (r) => {
            const body = JSON.parse(r.body);
            return body.author && body.author.name === '李白';
        },
    }) || errorRate.add(1);

    sleep(1);

    // Scenario 4: Search poems
    const searchRes = http.get(`${BASE_URL}/api/v1/poems/search?q=静夜思`);
    check(searchRes, {
        'search status 200': (r) => r.status === 200,
        'search has results': (r) => JSON.parse(r.body).pagination.total > 0,
    }) || errorRate.add(1);

    sleep(1);

    // Scenario 5: Get authors
    const authorsRes = http.get(`${BASE_URL}/api/v1/authors?page=1&page_size=20`);
    check(authorsRes, {
        'authors status 200': (r) => r.status === 200,
    }) || errorRate.add(1);

    sleep(1);
}
