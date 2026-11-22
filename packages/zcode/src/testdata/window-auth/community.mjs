export const CommunityZCodeFixture = async () => ({
    config(config) {
        config.provider ??= {};
        config.provider.zcode = {
            name: "ZCode community fixture",
            npm: "@ai-sdk/anthropic",
            models: { "GLM-5.3": { name: "GLM-5.3", limit: { context: 200000, output: 8192 } } },
        };
    },
    auth: {
        provider: "zcode",
        methods: [{ type: "api", label: "GLM Coding Plan API key" }],
        async loader(getAuth) {
            return {
                baseURL: "https://example.invalid/api/anthropic",
                async fetch() {
                    const selected = await getAuth();
                    if (selected?.type !== "oauth" || selected.uid !== "zcode-user") {
                        return Response.json({ type: "error", error: { message: "Wrong fixture account" } }, { status: 401 });
                    }
                    const events = [
                        { type: "message_start", message: { type: "message", id: "community-fixture", role: "assistant",
                            model: "GLM-5.3", content: [], usage: { input_tokens: 1, output_tokens: 0 } } },
                        { type: "content_block_start", index: 0, content_block: { type: "text", text: "community fixture" } },
                        { type: "content_block_stop", index: 0 },
                        { type: "message_delta", delta: { stop_reason: "end_turn" }, usage: { output_tokens: 1 } },
                        { type: "message_stop" },
                    ];
                    return new Response(events.map((event) => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join(""),
                        { headers: { "Content-Type": "text/event-stream" } });
                },
            };
        },
    },
});
