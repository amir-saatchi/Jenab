import { expect, test } from "bun:test"

import { hostProblem, withHost, withoutHost } from "@/shell/private-hosts"

test("hostProblem", () => {
  for (const ok of ["127.0.0.1", "nas.local", " localhost ", "::1", "[::1]", "fe80::1", "رایانه.local"]) {
    expect(hostProblem(ok)).toBeNull()
  }
  expect(hostProblem("  ")).toBe("Type a host.")
  expect(hostProblem("nas.local printer.local")).toBe("Add one host at a time.")
  expect(hostProblem("http://127.0.0.1")).toBe("Give the host alone, without http:// or a path.")
  expect(hostProblem("127.0.0.1/crypto")).toBe("Give the host alone, without http:// or a path.")
  expect(hostProblem("127.0.0.1:8099")).toBe("Give the host without a port.")
  expect(hostProblem("nas.local:")).toBe("Give the host without a port.")
  expect(hostProblem("[::1]:80")).toBe("Give the host without a port.")
})

test("withHost and withoutHost", () => {
  expect(withHost([], " 127.0.0.1 ")).toEqual(["127.0.0.1"])
  expect(withHost(["127.0.0.1"], "127.0.0.1")).toEqual(["127.0.0.1"])
  expect(withHost(["127.0.0.1"], "nas.local")).toEqual(["127.0.0.1", "nas.local"])
  expect(withoutHost(["127.0.0.1", "nas.local"], "127.0.0.1")).toEqual(["nas.local"])
  expect(withoutHost(["nas.local"], "other")).toEqual(["nas.local"])
})
