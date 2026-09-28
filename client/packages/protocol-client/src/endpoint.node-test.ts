import test from "node:test";
import assert from "node:assert/strict";

import {splitBaseURL} from "./transport.ts";

test("splitBaseURL separates the query and drops trailing slashes", () => {
    assert.deepEqual(splitBaseURL("http://h:5002"), {root: "http://h:5002", query: ""});
    assert.deepEqual(splitBaseURL("http://h:5002/"), {root: "http://h:5002", query: ""});
    assert.deepEqual(splitBaseURL("http://h:5002/?debug=1"), {root: "http://h:5002", query: "?debug=1"});
    assert.deepEqual(splitBaseURL("http://h/api?debug=1&x=a?b"), {root: "http://h/api", query: "?debug=1&x=a?b"});
});
