// Catálogo pt-BR, um arquivo por área (ADR-041). É ele que define o tipo Catalog e as
// chaves: o catálogo de cada outro idioma tem de ter exatamente a mesma forma.
import { common } from "./common";
import { nav } from "./nav";
import { theme } from "./theme";
import { lang } from "./lang";
import { login } from "./login";
import { profile } from "./profile";
import { app } from "./app";
import { ui } from "./ui";
import { actions } from "./actions";
import { api } from "./api";
import { dashboard } from "./dashboard";
import { health } from "./health";
import { nodes } from "./nodes";
import { torrents } from "./torrents";
import { add } from "./add";
import { explore } from "./explore";
import { downloaded } from "./downloaded";
import { analytics } from "./analytics";
import { feed } from "./feed";
import { alerts } from "./alerts";
import { audit } from "./audit";
import { users } from "./users";
import { settings } from "./settings";
import { trackers } from "./trackers";
import { publish } from "./publish";

export const ptBR = {
  common,
  nav,
  theme,
  lang,
  login,
  profile,
  app,
  ui,
  actions,
  api,
  dashboard,
  health,
  nodes,
  torrents,
  add,
  explore,
  downloaded,
  analytics,
  feed,
  alerts,
  audit,
  users,
  settings,
  trackers,
  publish,
} as const;
