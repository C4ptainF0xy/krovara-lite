<script lang="ts">
  import { ZoomIn } from '@lucide/svelte';
  import Modal from './Modal.svelte';
  import { Button } from '$lib/ui';

  type Shape = 'circle' | 'rounded' | 'rect';
  type Props = {
    open: boolean;
    file: File | null;
    busy?: boolean;
    title?: string;
    aspect?: number;
    shape?: Shape;
    outWidth?: number;
    onclose: () => void;
    oncropped: (blob: Blob) => void;
  };
  let {
    open,
    file,
    busy = false,
    title = "Recadrer l'image",
    aspect = 1,
    shape = 'circle',
    outWidth = 512,
    onclose,
    oncropped
  }: Props = $props();

  const STAGE_MAX = 320;
  const stageW = $derived(aspect >= 1 ? STAGE_MAX : Math.round(STAGE_MAX * aspect));
  const stageH = $derived(aspect >= 1 ? Math.round(STAGE_MAX / aspect) : STAGE_MAX);
  const outW = $derived(outWidth);
  const outH = $derived(Math.round(outWidth / aspect));
  const radius = $derived(shape === 'circle' ? '9999px' : shape === 'rounded' ? '16px' : '10px');

  let canvas = $state<HTMLCanvasElement | null>(null);
  let img: HTMLImageElement | null = null;
  let zoom = $state(1);
  let offset = $state({ x: 0, y: 0 });
  let baseScale = 1;
  let dragging = false;
  let dragStart = { x: 0, y: 0, ox: 0, oy: 0 };

  function coverScale() {
    if (!img) return 1;
    return Math.max(stageW / img.width, stageH / img.height);
  }

  $effect(() => {
    const f = file;
    if (!f) return;
    const url = URL.createObjectURL(f);
    const im = new Image();
    im.onload = () => {
      img = im;
      baseScale = coverScale();
      zoom = 1;
      offset = { x: 0, y: 0 };
      draw();
      URL.revokeObjectURL(url);
    };
    im.src = url;
  });

  function onZoom(e: Event) {
    zoom = Number((e.currentTarget as HTMLInputElement).value);
    draw();
  }

  function clampOffset() {
    if (!img) return;
    const w = img.width * baseScale * zoom;
    const h = img.height * baseScale * zoom;
    const maxX = Math.max(0, (w - stageW) / 2);
    const maxY = Math.max(0, (h - stageH) / 2);
    offset = {
      x: Math.min(maxX, Math.max(-maxX, offset.x)),
      y: Math.min(maxY, Math.max(-maxY, offset.y))
    };
  }

  function composeSource(
    src: CanvasImageSource,
    srcW: number,
    srcH: number,
    ctx: CanvasRenderingContext2D,
    k: number
  ) {
    const cw = stageW * k;
    const ch = stageH * k;
    const w = srcW * baseScale * zoom * k;
    const h = srcH * baseScale * zoom * k;
    ctx.clearRect(0, 0, cw, ch);
    ctx.drawImage(src, (cw - w) / 2 + offset.x * k, (ch - h) / 2 + offset.y * k, w, h);
  }

  function compose(ctx: CanvasRenderingContext2D, k: number) {
    if (img) composeSource(img, img.width, img.height, ctx, k);
  }

  const isGif = $derived(file?.type === 'image/gif');

  function draw() {
    if (!canvas || !img) return;
    clampOffset();
    const ctx = canvas.getContext('2d');
    if (ctx) compose(ctx, 1);
  }

  function onPointerDown(e: PointerEvent) {
    dragging = true;
    dragStart = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging) return;
    offset = { x: dragStart.ox + (e.clientX - dragStart.x), y: dragStart.oy + (e.clientY - dragStart.y) };
    draw();
  }
  function onPointerUp() {
    dragging = false;
  }

  function save() {
    if (!img) return;
    if (isGif) {
      void saveGif();
      return;
    }
    const out = document.createElement('canvas');
    out.width = outW;
    out.height = outH;
    const ctx = out.getContext('2d');
    if (!ctx) return;
    compose(ctx, outW / stageW);
    out.toBlob((blob) => blob && oncropped(blob), 'image/png');
  }

  let gifBusy = $state(false);
  async function saveGif() {
    if (!file) return;
    gifBusy = true;
    try {
      const [{ parseGIF, decompressFrames }, gifenc] = await Promise.all([
        import('gifuct-js'),
        import('gifenc')
      ]);
      const buf = await file.arrayBuffer();
      const gif = parseGIF(buf);
      const frames = decompressFrames(gif, true);
      const W = gif.lsd.width;
      const H = gif.lsd.height;

      const full = document.createElement('canvas');
      full.width = W;
      full.height = H;
      const fctx = full.getContext('2d');
      const patchC = document.createElement('canvas');
      const out = document.createElement('canvas');
      out.width = outW;
      out.height = outH;
      const octx = out.getContext('2d');
      if (!fctx || !octx) return;

      const enc = gifenc.GIFEncoder();
      const k = outW / stageW;
      let prevDisposal = 0;
      let prevDims: { left: number; top: number; width: number; height: number } | null = null;
      let restore: ImageData | null = null;

      for (const fr of frames) {
        if (prevDisposal === 2 && prevDims) {
          fctx.clearRect(prevDims.left, prevDims.top, prevDims.width, prevDims.height);
        } else if (prevDisposal === 3 && restore) {
          fctx.putImageData(restore, 0, 0);
        }
        if (fr.disposalType === 3) restore = fctx.getImageData(0, 0, W, H);

        patchC.width = fr.dims.width;
        patchC.height = fr.dims.height;
        const pctx = patchC.getContext('2d');
        if (!pctx) return;
        pctx.putImageData(
          new ImageData(new Uint8ClampedArray(fr.patch), fr.dims.width, fr.dims.height),
          0,
          0
        );
        fctx.drawImage(patchC, fr.dims.left, fr.dims.top);
        prevDisposal = fr.disposalType;
        prevDims = fr.dims;

        composeSource(full, W, H, octx, k);
        const { data } = octx.getImageData(0, 0, outW, outH);
        const palette = gifenc.quantize(data, 256);
        const index = gifenc.applyPalette(data, palette);
        enc.writeFrame(index, outW, outH, { palette, delay: fr.delay || 100 });
      }
      enc.finish();
      oncropped(new Blob([enc.bytesView()], { type: 'image/gif' }));
    } catch {
      const out = document.createElement('canvas');
      out.width = outW;
      out.height = outH;
      const ctx = out.getContext('2d');
      if (ctx) {
        compose(ctx, outW / stageW);
        out.toBlob((blob) => blob && oncropped(blob), 'image/png');
      }
    } finally {
      gifBusy = false;
    }
  }
</script>

<Modal {open} {title} {onclose}>
  <div class="flex flex-col items-center gap-4">
    <div class="relative shrink-0 overflow-hidden rounded-lg bg-base" style="width:{stageW}px;height:{stageH}px">
      <canvas
        bind:this={canvas}
        width={stageW}
        height={stageH}
        class="touch-none cursor-grab active:cursor-grabbing"
        onpointerdown={onPointerDown}
        onpointermove={onPointerMove}
        onpointerup={onPointerUp}
        onpointercancel={onPointerUp}
      ></canvas>
      <div
        class="pointer-events-none absolute inset-0"
        style="box-shadow: inset 0 0 0 9999px rgba(15,15,20,0.6); border-radius:{radius};"
      ></div>
      <div class="pointer-events-none absolute inset-0 ring-2 ring-brand/70" style="border-radius:{radius};"></div>
    </div>

    <div class="flex w-full items-center gap-3">
      <ZoomIn size={18} class="shrink-0 text-muted" />
      <input
        type="range"
        min="1"
        max="3"
        step="0.01"
        value={zoom}
        oninput={onZoom}
        class="h-1 w-full cursor-pointer appearance-none rounded-full bg-elevated accent-brand"
        aria-label="Zoom"
      />
    </div>
    <p class="text-label text-muted">Glisse pour repositionner, ajuste le zoom.</p>

    <div class="flex w-full justify-end gap-2">
      <Button type="button" variant="ghost" onclick={onclose}>Annuler</Button>
      <Button type="button" loading={busy || gifBusy} onclick={save}>
        {gifBusy ? 'Encodage du GIF…' : 'Enregistrer'}
      </Button>
    </div>
  </div>
</Modal>
