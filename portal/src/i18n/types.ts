import type { ptBR } from "./pt-BR";

// Plural: object with the Intl.PluralRules categories (one/other in pt-BR and en).
export type Plural = { one: string; other: string };

// Widens the leaves of ptBR to `string`/`Plural`, so that `en` satisfies the type
// without depending on `as const` — but the key structure must be exactly the same.
export type Catalog = CatalogShape<typeof ptBR>;
type CatalogShape<T> = { [K in keyof T]: Leaf<T[K]> };
type Leaf<T> = T extends Plural ? Plural : T extends string ? string : CatalogShape<T>;
