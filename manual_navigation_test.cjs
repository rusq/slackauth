const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const script = fs.readFileSync(path.join(__dirname, "manual_navigation.js"), "utf8");

test("manual navigation preserves the Slack browser fallback", () => {
    for (const [origin, hasNavigation, listens] of [
        ["https://example.slack.com", true, true],
        ["https://slack.com", true, true],
        ["http://example.slack.com", true, false],
        ["https://slack.com.example.org", true, false],
        ["https://notslack.com", true, false],
        ["https://example.org", true, false],
        ["https://example.slack.com", false, false],
    ]) {
        let listener;
        const navigation = {
            addEventListener(name, callback) {
                assert.equal(name, "navigate");
                listener = callback;
            },
        };
        vm.runInNewContext(script, {
            location: new URL(origin),
            window: { navigation: hasNavigation ? navigation : undefined },
            URL,
        });
        assert.equal(typeof listener === "function", listens, origin);
        if (!listener) continue;

        for (const [url, cancelable, cancels] of [
            ["slack://synthetic-navigation", true, true],
            ["SLACK://synthetic-navigation", true, true],
            ["slack://synthetic-navigation", false, false],
            ["https://example.slack.com/client", true, false],
            ["https://identity.example.org", true, false],
            ["mailto:synthetic@example.org", true, false],
            ["about:blank", true, false],
        ]) {
            let prevented = false;
            listener({
                destination: { url },
                cancelable,
                preventDefault() { prevented = true; },
            });
            assert.equal(prevented, cancels, `${origin}: ${url}, cancelable=${cancelable}`);
        }
    }
});
