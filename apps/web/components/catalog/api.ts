export type Difficulty = "beginner" | "intermediate" | "advanced";

export type Problem = {
  slug: string;
  version: number;
  title: string;
  summary: string;
  language: "go";
  difficulty: Difficulty;
  tags?: string[];
};

export type ProblemPage = {
  items: Problem[];
  nextCursor?: string | null;
};

export type CatalogResult<T> =
  | { status: "ready"; data: T }
  | { status: "unavailable" }
  | { status: "not-found" };

const difficulties = new Set(["beginner", "intermediate", "advanced"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isProblem(value: unknown): value is Problem {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    Number.isInteger(value.version) &&
    typeof value.title === "string" &&
    typeof value.summary === "string" &&
    value.language === "go" &&
    difficulties.has(value.difficulty as string) &&
    (value.tags === undefined ||
      (Array.isArray(value.tags) && value.tags.every((tag) => typeof tag === "string")))
  );
}

function isProblemPage(value: unknown): value is ProblemPage {
  return (
    isRecord(value) &&
    Array.isArray(value.items) &&
    value.items.every(isProblem) &&
    (value.nextCursor === undefined || value.nextCursor === null || typeof value.nextCursor === "string")
  );
}

async function get(path: string): Promise<CatalogResult<unknown>> {
  const base = process.env.API_INTERNAL_URL ?? "http://127.0.0.1:4000";

  try {
    const url = new URL(path, new URL("/", base));
    const response = await fetch(url, {
      next: { revalidate: 30 },
      signal: AbortSignal.timeout(5000),
    });
    if (response.status === 404) return { status: "not-found" };
    if (!response.ok) return { status: "unavailable" };
    return { status: "ready", data: await response.json() };
  } catch {
    return { status: "unavailable" };
  }
}

export async function listProblems(cursor?: string): Promise<CatalogResult<ProblemPage>> {
  const query = new URLSearchParams({ limit: "12" });
  if (cursor) query.set("cursor", cursor);
  const result = await get(`/api/v1/problems?${query}`);
  if (result.status !== "ready") return result;
  return isProblemPage(result.data)
    ? { status: "ready", data: result.data }
    : { status: "unavailable" };
}

export async function getProblem(slug: string): Promise<CatalogResult<Problem>> {
  const result = await get(`/api/v1/problems/${encodeURIComponent(slug)}`);
  if (result.status !== "ready") return result;
  return isProblem(result.data)
    ? { status: "ready", data: result.data }
    : { status: "unavailable" };
}
