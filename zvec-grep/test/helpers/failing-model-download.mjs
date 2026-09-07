const originalFetch = globalThis.fetch;

globalThis.fetch = async (input, init) => {
  const url =
    typeof Request !== "undefined" && input instanceof Request
      ? input.url
      : String(input);
  if (!url.startsWith("https://huggingface.co/")) {
    return await originalFetch(input, init);
  }

  throw new Error(
    "simulated model download network failure token=model-download-secret",
  );
};
