# Journal search uses trigram substring matching, not english full-text

The Journal's search was backed by a Postgres `tsvector` generated with the
`english` text-search configuration (`to_tsvector('english', body)`) queried
via `websearch_to_tsquery`. That configuration tokenizes on whitespace and
punctuation, so it cannot segment CJK text: an entire run of Chinese collapses
into a single token. Searching for a substring like `个人` therefore never
matched, even though the characters were present — the index stored the whole
phrase as one indivisible lexeme and `tsquery` matches whole lexemes, not
substrings.

Installing a CJK tokenizer (`zhparser` / `pg_jieba`) would give true
word-based search, but those extensions are not available on our Postgres
server and require server-side installation plus a matching database image for
local/CI. For a single-user Journal with a small corpus, that cost is not
justified.

Instead the Journal matches entries by case-insensitive substring using
`body ILIKE '%term%'`, backed by a GIN trigram index (`pg_trgm`, already
available). This is language-agnostic and finds Chinese, English, and mixed
text alike. The trade-offs we accept:

- No stemming, stop-words, or relevance ranking — results are newest-first.
- Matching is substring, not word-based, so a query can match across word
  boundaries (minor over-recall).
- Terms of 1–2 characters produce few trigrams and are poorly selective; the
  index degrades toward a scan + recheck. At the Journal's scale this is
  negligible.

If the corpus ever grows large enough to need word-based retrieval and
ranking, revisit with a CJK-aware full-text configuration (requires the
tokenizer extension on the server).
