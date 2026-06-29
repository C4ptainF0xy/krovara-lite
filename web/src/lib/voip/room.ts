import { writable, type Readable } from 'svelte/store';
import { hangupRoom, setRoomMembership } from './signaling';
import { joinRoom as sfuJoin, leaveRoom as sfuLeave, sfuState } from './sfu';
import { voiceMode } from './mode';

const _activeRoom = writable<string>('');
export const activeRoom: Readable<string> = { subscribe: _activeRoom.subscribe };

let current = '';
_activeRoom.subscribe((v) => (current = v));

if (voiceMode === 'sfu') {
  sfuState.subscribe((s) => {
    if (s === 'idle' && current) _activeRoom.set('');
  });
}

export async function joinVoice(channelId: string): Promise<void> {
  if (current === channelId) return;
  if (current) await leaveVoice();
  if (voiceMode === 'sfu') {
    await sfuJoin(channelId);
  } else {
    setRoomMembership(channelId, true);
  }
  _activeRoom.set(channelId);
}

export async function leaveVoice(): Promise<void> {
  if (!current) return;
  const room = current;
  if (voiceMode === 'sfu') {
    await sfuLeave();
  } else {
    setRoomMembership(room, false);
    await hangupRoom(room);
  }
  _activeRoom.set('');
}
