import rawGroups from 'unicode-emoji-json/data-by-group.json';

export type EmojiEntry = { e: string; k: string };
export type EmojiCategory = { id: string; label: string; icon: string; items: EmojiEntry[] };

type RawEmoji = { emoji: string; name: string; slug: string };
type RawGroup = { name: string; slug: string; emojis: RawEmoji[] };

const norm = (s: string) => s.replace(/️/g, '');

const FR_ALIASES_RAW: Record<string, string> = {
  '😀': 'sourire grin content happy',
  '😁': 'sourire dents content beam',
  '😂': 'rire larmes joy lol mdr',
  '🤣': 'rire roule rofl mdr',
  '😊': 'content rougir blush timide',
  '🙂': 'sourire leger slight',
  '😉': 'clin oeil wink',
  '😍': 'amour yeux coeur love heart eyes',
  '🥰': 'amour coeurs adore love',
  '😘': 'bisou kiss embrasse',
  '😎': 'cool lunettes sunglasses',
  '🤩': 'star etoiles wow excite',
  '🤔': 'reflechir pense thinking hmm',
  '😴': 'dort sommeil sleep zzz',
  '😢': 'triste pleure cry sad',
  '😭': 'pleure sanglot sob cry',
  '😡': 'colere fache angry rage',
  '🥳': 'fete party celebration',
  '😅': 'sueur nerveux sweat',
  '😏': 'malicieux smirk sourire',
  '🙄': 'yeux ciel roll exasperer',
  '😬': 'grimace crispe',
  '🤯': 'explose mind blown choque',
  '🥺': 'supplie pleading yeux',
  '👍': 'pouce oui ok thumbs up like',
  '👎': 'pouce bas non dislike',
  '👏': 'applaudir clap bravo',
  '🙏': 'priere merci please thanks',
  '🤝': 'poignee main deal accord',
  '👋': 'salut coucou wave hello',
  '✌️': 'paix victoire peace',
  '🤞': 'doigts croises luck chance',
  '🤙': 'appelle shaka',
  '💪': 'muscle fort strong',
  '🫡': 'salut militaire salute',
  '🙌': 'mains levees hooray',
  '👀': 'yeux regarde eyes look',
  '🫶': 'coeur mains love',
  '👌': 'ok parfait perfect',
  '🤌': 'italien pincee',
  '❤️': 'coeur rouge love heart',
  '🧡': 'coeur orange',
  '💛': 'coeur jaune',
  '💚': 'coeur vert',
  '💙': 'coeur bleu',
  '💜': 'coeur violet purple',
  '🖤': 'coeur noir black',
  '🤍': 'coeur blanc white',
  '💖': 'coeur brillant sparkle',
  '💕': 'coeurs deux love',
  '💔': 'coeur brise broken',
  '❣️': 'coeur exclamation',
  '🐱': 'chat cat',
  '🐶': 'chien dog',
  '🦊': 'renard fox',
  '🐼': 'panda',
  '🐸': 'grenouille frog',
  '🦁': 'lion',
  '🐧': 'pingouin penguin',
  '🦄': 'licorne unicorn',
  '🐝': 'abeille bee',
  '🌟': 'etoile star brille',
  '🔥': 'feu fire flamme hot',
  '🌈': 'arc ciel rainbow',
  '🌸': 'fleur blossom',
  '🌺': 'fleur hibiscus',
  '🍀': 'trefle luck chance',
  '⭐': 'etoile star',
  '🍕': 'pizza',
  '🍔': 'burger hamburger',
  '🍟': 'frites fries',
  '🌮': 'taco',
  '🍣': 'sushi',
  '🍜': 'ramen nouilles noodles',
  '🍩': 'donut beignet',
  '🍪': 'cookie biscuit',
  '🎂': 'gateau cake anniversaire',
  '🍫': 'chocolat chocolate',
  '☕': 'cafe coffee',
  '🍺': 'biere beer',
  '🥂': 'champagne trinque cheers',
  '🍷': 'vin wine',
  '🍎': 'pomme apple',
  '🍓': 'fraise strawberry',
  '🎮': 'jeu manette game gaming',
  '🕹️': 'joystick arcade',
  '🎲': 'de dice hasard',
  '🏆': 'trophee trophy victoire win',
  '🥇': 'medaille or gold premier',
  '⚽': 'foot football soccer',
  '🏀': 'basket basketball',
  '🎯': 'cible dart target',
  '🎸': 'guitare guitar musique',
  '🎧': 'casque headphones musique',
  '🎉': 'fete party tada confetti',
  '🎊': 'confetti fete',
  '🎁': 'cadeau gift present',
  '🎤': 'micro karaoke sing',
  '🏁': 'arrivee finish course',
  '🚀': 'fusee rocket lancement',
  '💡': 'idee ampoule idea light',
  '💻': 'ordinateur laptop pc',
  '📱': 'telephone phone',
  '⌨️': 'clavier keyboard',
  '📷': 'appareil photo camera',
  '🔋': 'batterie battery',
  '💰': 'argent money sac',
  '📌': 'epingle pin punaise',
  '📎': 'trombone clip',
  '✏️': 'crayon pencil ecrire',
  '📚': 'livres books',
  '🔑': 'cle key',
  '🔒': 'cadenas lock verrou',
  '⏰': 'reveil alarm horloge',
  '🎵': 'note musique music',
  '📣': 'megaphone annonce',
  '✅': 'check oui valide ok done',
  '❌': 'croix non erreur cross no',
  '⚠️': 'attention warning danger',
  '❓': 'question interrogation',
  '❗': 'exclamation important',
  '💯': 'cent parfait hundred',
  '✨': 'etincelles sparkles brille',
  '➡️': 'fleche droite arrow right',
  '🔔': 'cloche bell notification',
  '🔕': 'cloche muet mute silence',
  '♻️': 'recyclage recycle',
  '🆗': 'ok',
  '🆕': 'nouveau new',
  '🔴': 'rond rouge red',
  '🟢': 'rond vert green'
};

const FR_ALIASES: Record<string, string> = {};
for (const [k, v] of Object.entries(FR_ALIASES_RAW)) FR_ALIASES[norm(k)] = v;

const GROUP_META: Record<string, { id: string; label: string; icon: string }> = {
  'Smileys & Emotion': { id: 'smileys', label: 'Émotions', icon: '😀' },
  'People & Body': { id: 'people', label: 'Personnes', icon: '👋' },
  'Animals & Nature': { id: 'nature', label: 'Nature', icon: '🐱' },
  'Food & Drink': { id: 'food', label: 'Nourriture', icon: '🍕' },
  'Travel & Places': { id: 'travel', label: 'Voyage', icon: '✈️' },
  Activities: { id: 'activity', label: 'Activités', icon: '🎮' },
  Objects: { id: 'objects', label: 'Objets', icon: '💡' },
  Symbols: { id: 'symbols', label: 'Symboles', icon: '✅' },
  Flags: { id: 'flags', label: 'Drapeaux', icon: '🏳️' }
};

export const EMOJI_CATEGORIES: EmojiCategory[] = (rawGroups as RawGroup[])
  .filter((g) => GROUP_META[g.name])
  .map((g) => {
    const meta = GROUP_META[g.name];
    return {
      id: meta.id,
      label: meta.label,
      icon: meta.icon,
      items: g.emojis.map((em) => {
        const alias = FR_ALIASES[norm(em.emoji)];
        const k = `${em.name} ${em.slug.replace(/_/g, ' ')}${alias ? ` ${alias}` : ''}`.toLowerCase();
        return { e: em.emoji, k };
      })
    };
  });

const ALL: EmojiEntry[] = EMOJI_CATEGORIES.flatMap((c) => c.items);

export function searchEmoji(query: string): EmojiEntry[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  return ALL.filter((it) => it.k.includes(q) || it.e === q).slice(0, 64);
}
