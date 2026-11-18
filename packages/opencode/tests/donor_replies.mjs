import { writeFile } from "node:fs/promises";
import { toolReply } from "../src/testdata/donors/tool-fixtures.mjs";

const fixtures = Object.fromEntries(["chat", "responses", "messages"].map(protocol => {
  const donor = protocol === "messages" ? "anthropic" : protocol;
  return [protocol, { json: toolReply(donor, "Read", false), sse: toolReply(donor, "Read", true) }];
}));
await writeFile(process.argv[2], JSON.stringify(fixtures, null, 2) + "\n");
