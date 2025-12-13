import http from 'k6/http';
import { check } from 'k6';

// Spike test - sudden traffic surge
export const options = {
    stages: [
        { duration: '10s', target: 100 },   // Normal load
        { duration: '10s', target: 1000 },  // Sudden spike!
        { duration: '30s', target: 1000 },  // Stay at spike
        { duration: '10s', target: 100 },   // Back to normal
        { duration: '10s', target: 0 },     // Ramp down
    ],
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:1279';

export default function () {
    const res = http.get(`${BASE_URL}/api/v1/poems/random`);
    check(res, {
        'status is 200': (r) => r.status === 200,
    });
}
