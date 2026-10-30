const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function script(name) {
    const html = fs.readFileSync(path.join(__dirname, '..', 'templates', name), 'utf8');
    return html.match(/<script>([\s\S]*?)<\/script>/)[1];
}

const settle = () => new Promise(resolve => setImmediate(resolve));

test('widget retries missing data and backend failures with a delay', async () => {
    let calls = 0;
    let resume;
    let start;
    const number = {innerText: ''};
    const delays = [];
    const context = {
        console: {log() {}, error() {}},
        document: {
            location: {protocol: 'https:'},
            documentElement: {style: {setProperty() {}}},
            getElementById: () => number,
        },
        window: {
            location: {pathname: '/uuid/widget/view/public-id', origin: 'https://test.invalid'},
            addEventListener(event, callback) {
                assert.equal(event, 'DOMContentLoaded');
                start = callback;
            },
        },
        fetch: async () => {
            calls++;
            if (calls === 1) return {ok: false, json: async () => ({message: 'No data'})};
            if (calls === 2 || calls === 5) return {ok: true, json: async () => ({data: {heart_rate: 72}})};
            if (calls === 3) return {ok: false, json: async () => ({message: 'Backend unavailable'})};
            throw new Error('Network unavailable');
        },
        setTimeout(callback, milliseconds) {
            delays.push(milliseconds);
            resume = callback;
        },
    };
    vm.runInNewContext(script('default.html'), context);
    assert.equal(calls, 0);
    start();
    const expected = ['--', 72, '--', '--', 72];
    for (let i = 0; i < expected.length; i++) {
        await settle();
        assert.equal(calls, i + 1);
        assert.equal(number.innerText, expected[i]);
        assert.equal(delays[i], 1000);
        if (i < expected.length - 1) resume();
    }
});

test('login UI keeps public links separate from the private reporting link', async () => {
    const elements = {};
    const context = {
        window: {location: {origin: 'https://test.invalid'}},
        document: {getElementById(id) { return elements[id] ||= {style: {}}; }},
        fetch: async () => ({ok: true, json: async () => ({data: {uuid: 'public-id', report_token: 'private-token'}})}),
        console,
    };
    vm.createContext(context);
    vm.runInContext(script('auth.html'), context);
    await vm.runInContext('fetchUserUUID()', context);
    assert.equal(elements['report-url'].value, 'https://test.invalid/uuid/private-token/receive_data');
    assert.equal(elements['view-url'].value, 'https://test.invalid/uuid/widget/view/public-id');
    assert.equal(elements['latest-url'].value, 'https://test.invalid/uuid/public-id/latest-heart-rate');
    assert.equal(elements['user-uuid'].value, 'public-id');
});
