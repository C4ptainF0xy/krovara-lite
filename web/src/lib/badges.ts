export type BadgeMeta = { key: string; label: string; img: string };

export const BADGE_LIST: BadgeMeta[] = [
  { key: 'founder', label: 'Fondateur', img: '/badges/73399-founder.png' },
  { key: 'owner', label: 'Propriétaire', img: '/badges/46501-owner.png' },
  { key: 'staff', label: 'Équipe Krovara', img: '/badges/krovarastaff.png' },
  { key: 'admin', label: 'Administrateur', img: '/badges/80271-admin.png' },
  { key: 'moderator', label: 'Modérateur', img: '/badges/60688-moderator.png' },
  { key: 'trial_mod', label: 'Modérateur (essai)', img: '/badges/52662-trial-mod.png' },
  { key: 'developer', label: 'Développeur', img: '/badges/58892-developer.png' },
  { key: 'designer', label: 'Designer', img: '/badges/36438-designer.png' },
  { key: 'contributor', label: 'Contributeur', img: '/badges/69705-editor.png' },
  { key: 'bug_hunter', label: 'Chasseur de bugs', img: '/badges/11838-warning.png' },
  { key: 'supporter', label: 'Soutien', img: '/badges/61272-support.png' },
  { key: 'booster', label: 'Booster', img: '/badges/4642-booster.png' },
  { key: 'partner', label: 'Partenaire', img: '/badges/93771-twitch-partner.png' },
  { key: 'verified', label: 'Vérifié', img: '/badges/85185-verified.png' },
  { key: 'early', label: 'Premier arrivé', img: '/badges/74135-new-member.png' },
  { key: 'vip', label: 'VIP', img: '/badges/34866-diamond.png' },
  { key: 'bot', label: 'Bot', img: '/badges/95805-bot.png' }
];

export const BADGES: Record<string, BadgeMeta> = Object.fromEntries(
  BADGE_LIST.map((b) => [b.key, b])
);

const BADGE_RANK: Record<string, number> = Object.fromEntries(
  BADGE_LIST.map((b, i) => [b.key, i])
);

export function sortBadges(keys: string[] | null | undefined): string[] {
  if (!keys) return [];
  return keys
    .filter((k) => k in BADGE_RANK)
    .sort((a, b) => BADGE_RANK[a] - BADGE_RANK[b]);
}
