export type VoiceMode = 'sfu' | 'mesh';

export const voiceMode: VoiceMode =
  (import.meta.env.VITE_KROVARA_VOIP_MODE as VoiceMode | undefined) === 'mesh'
    ? 'mesh'
    : 'sfu';
