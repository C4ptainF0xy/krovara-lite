export const PERMISSIONS = {
  ViewChannel: 1 << 0,
  SendMessages: 1 << 1,
  ManageMessages: 1 << 2,
  ManageChannels: 1 << 3,
  ManageRoles: 1 << 4,
  KickMembers: 1 << 5,
  BanMembers: 1 << 6,
  ManageSpace: 1 << 7,
  CreateInvite: 1 << 8,
  ConnectVoice: 1 << 9,
  SpeakVoice: 1 << 10,
  Administrator: 1 << 11
} as const;

export const CHANNEL_OVERWRITE_PERMS: { bit: number; label: string }[] = [
  { bit: PERMISSIONS.ViewChannel, label: 'Voir le salon' },
  { bit: PERMISSIONS.SendMessages, label: 'Envoyer des messages' },
  { bit: PERMISSIONS.ManageMessages, label: 'Gérer les messages' },
  { bit: PERMISSIONS.CreateInvite, label: 'Créer une invitation' },
  { bit: PERMISSIONS.ConnectVoice, label: 'Se connecter au vocal' },
  { bit: PERMISSIONS.SpeakVoice, label: 'Parler dans le vocal' }
];

export type PermDef = { bit: number; label: string; desc?: string; danger?: boolean };
export const PERMISSION_GROUPS: { title: string; perms: PermDef[] }[] = [
  {
    title: 'Général',
    perms: [
      { bit: PERMISSIONS.ManageSpace, label: 'Gérer l’espace', desc: 'Nom, branding, paramètres.' },
      { bit: PERMISSIONS.ManageRoles, label: 'Gérer les rôles', desc: 'Créer/éditer les rôles sous le sien.' },
      { bit: PERMISSIONS.ManageChannels, label: 'Gérer les salons', desc: 'Créer, éditer, supprimer.' },
      { bit: PERMISSIONS.CreateInvite, label: 'Créer une invitation' },
      {
        bit: PERMISSIONS.Administrator,
        label: 'Administrateur',
        desc: 'Toutes les permissions. À donner avec prudence.',
        danger: true
      }
    ]
  },
  {
    title: 'Membres',
    perms: [
      { bit: PERMISSIONS.KickMembers, label: 'Expulser des membres' },
      { bit: PERMISSIONS.BanMembers, label: 'Bannir des membres' }
    ]
  },
  {
    title: 'Salon texte',
    perms: [
      { bit: PERMISSIONS.ViewChannel, label: 'Voir les salons' },
      { bit: PERMISSIONS.SendMessages, label: 'Envoyer des messages' },
      { bit: PERMISSIONS.ManageMessages, label: 'Gérer les messages', desc: 'Épingler, supprimer ceux des autres.' }
    ]
  },
  {
    title: 'Vocal',
    perms: [
      { bit: PERMISSIONS.ConnectVoice, label: 'Se connecter' },
      { bit: PERMISSIONS.SpeakVoice, label: 'Parler' }
    ]
  }
];

export function hasBit(perms: number, bit: number): boolean {
  return (perms & bit) === bit;
}

export type TriState = 'inherit' | 'allow' | 'deny';

export function triStateOf(bit: number, allow: number, deny: number): TriState {
  if ((allow & bit) === bit) return 'allow';
  if ((deny & bit) === bit) return 'deny';
  return 'inherit';
}
