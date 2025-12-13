import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

// SAFE gradual ramp-up test
// Start small and gradually increase load
export const options = {
    stages: [
        // Phase 1: Warm up (verify everything works)
        { duration: '30s', target: 10 },    // Start with just 10 users
        { duration: '30s', target: 10 },    // Hold at 10 users

        // Phase 2: Light load
        { duration: '1m', target: 50 },     // Ramp to 50 users
        { duration: '1m', target: 50 },     // Hold at 50 users

        // Phase 3: Moderate load
        { duration: '1m', target: 100 },    // Ramp to 100 users
        { duration: '2m', target: 100 },    // Hold at 100 users

        // Phase 4: Heavy load (watch carefully!)
        { duration: '1m', target: 200 },    // Ramp to 200 users
        { duration: '1m', target: 200 },    // Hold at 200 users

        // Phase 5: Cool down
        { duration: '30s', target: 0 },     // Ramp down
    ],

    // Safety thresholds - test will ABORT if these are violated
    thresholds: {
        // Abort if error rate > 10%
        'errors': [
            { threshold: 'rate<0.1', abortOnFail: true },
        ],
        // Abort if p95 latency > 5 seconds
        'http_req_duration': [
            { threshold: 'p(95)<5000', abortOnFail: true },
        ],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:1279';

export default function () {
    const res = http.get(`${BASE_URL}/api/v1/poems/random`);

    const success = check(res, {
        'status is 200': (r) => r.status === 200,
        'response time < 5s': (r) => r.timings.duration < 5000,
    });

    if (!success) {
        errorRate.add(1);
    }

    sleep(1);
}

// Lifecycle hooks for monitoring
export function setup() {
    console.log('🚀 Starting safe load test...');
    console.log('⚠️  Test will auto-abort if:');
    console.log('   - Error rate > 10%');
    console.log('   - P95 latency > 5s');
    console.log('');
    console.log('💡 Monitor your server with: htop, docker stats, or system monitor');
    console.log('');
}

export function teardown(data) {
    console.log('');
    console.log('✅ Test completed safely');
}
