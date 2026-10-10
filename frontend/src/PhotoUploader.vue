<script setup lang="ts">
import { ref } from "vue";
import { Camera, Upload, X } from "lucide-vue-next";
import { api } from "./api";
const props = defineProps<{
  assetId: number;
  purpose: string;
  modelValue: number[];
}>();
const emit = defineEmits<{
  "update:modelValue": [number[]];
  uploaded: [];
  busy: [boolean];
}>();
const busy = ref(false),
  error = ref(""),
  caption = ref("");
const uploaded = ref<{ id: number; name: string }[]>([]);
async function upload(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  busy.value = true;
  emit("busy", true);
  error.value = "";
  const ids = [...props.modelValue];
  try {
    for (const file of files) {
      if (
        !["image/jpeg", "image/png"].includes(file.type) ||
        file.size > 10 * 1024 * 1024
      )
        throw new Error("Gunakan foto JPEG/PNG maksimal 10 MB.");
      const bitmap = await createImageBitmap(file);
      const scale = Math.min(1, 1600 / Math.max(bitmap.width, bitmap.height));
      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, Math.round(bitmap.width * scale));
      canvas.height = Math.max(1, Math.round(bitmap.height * scale));
      const context = canvas.getContext("2d");
      if (!context) throw new Error("Foto tidak dapat diproses");
      context.fillStyle = "#ffffff";
      context.fillRect(0, 0, canvas.width, canvas.height);
      context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
      bitmap.close();
      let quality = 0.8,
        data = canvas.toDataURL("image/jpeg", quality).split(",")[1];
      while (data.length > 690000 && quality > 0.25) {
        quality -= 0.1;
        data = canvas.toDataURL("image/jpeg", quality).split(",")[1];
      }
      if (data.length > 690000)
        throw new Error("Foto terlalu besar setelah kompresi.");
      const result = await api(`/assets/${props.assetId}/photos`, {
        purpose: props.purpose,
        caption: caption.value || file.name.slice(0, 200),
        data,
      });
      uploaded.value.push({ id: result.id, name: file.name });
      ids.push(result.id);
      emit("update:modelValue", [...ids]);
      emit("uploaded");
    }
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
    emit("busy", false);
    input.value = "";
  }
}
function unselect(id: number) {
  emit(
    "update:modelValue",
    props.modelValue.filter((value) => value !== id),
  );
  uploaded.value = uploaded.value.filter((photo) => photo.id !== id);
}
</script>
<template>
  <div class="photo-upload">
    <label>Caption foto<input v-model="caption" maxlength="200" /></label>
    <div class="photo-controls">
      <label
        class="secondary upload-button"
        :class="{ disabled: busy }"
        title="Unggah foto"
        ><Upload :size="17" /><span>Unggah foto</span
        ><input
          type="file"
          accept="image/jpeg,image/png"
          multiple
          :disabled="busy"
          @change="upload" /></label
      ><label class="secondary upload-button" title="Ambil foto"
        ><Camera :size="17" /><span>Kamera</span
        ><input
          type="file"
          accept="image/jpeg,image/png"
          capture="environment"
          :disabled="busy"
          @change="upload" /></label
      ><span v-if="busy" role="status">Mengunggah...</span>
    </div>
    <div class="evidence-thumbnails">
      <figure v-for="photo in uploaded" :key="photo.id">
        <img :src="`/api/photos/${photo.id}`" :alt="photo.name" /><button
          type="button"
          class="icon"
          title="Lepaskan dari formulir"
          aria-label="Lepaskan foto dari formulir"
          @click="unselect(photo.id)"
        >
          <X :size="14" />
        </button>
      </figure>
    </div>
    <p v-if="error" class="alert" role="alert">{{ error }}</p>
  </div>
</template>
<style scoped>
.photo-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin: 10px 0;
}
.upload-button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  cursor: pointer;
  position: relative;
  border: 1px solid var(--line);
  border-radius: 6px;
}
.upload-button input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}
.upload-button:focus-within {
  outline: 2px solid var(--focus);
}
.disabled {
  opacity: 0.5;
  pointer-events: none;
}
.evidence-thumbnails {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.evidence-thumbnails figure {
  position: relative;
  margin: 0;
  width: 96px;
  height: 76px;
}
.evidence-thumbnails img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: var(--surface-muted);
  border: 1px solid var(--line);
  border-radius: 4px;
}
.evidence-thumbnails button {
  position: absolute;
  top: 2px;
  right: 2px;
  background: var(--surface);
}
</style>
