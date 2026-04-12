<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {Events} from "@wailsio/runtime";
import {App} from "../../bindings/github.com/library-squirrel/wails";

defineProps<{ msg: string }>()

const name = ref('')
const result = ref('Please enter your name below 👋')
const time = ref('Listening for Time event...')
const version = ref('')

const doGreet = () => {
  let localName = name.value;
  if (!localName) {
    localName = 'anonymous';
  }
  App.Greet(localName).then((resultValue: string) => {
    result.value = resultValue;
  }).catch((err: Error) => {
    console.log(err);
  });
}

const loadVersion = () => {
  App.GetVersion().then((v: string) => {
    version.value = v;
  }).catch((err: Error) => {
    console.log(err);
  });
}

onMounted(() => {
  Events.On('time', (timeValue: { data: string }) => {
    time.value = timeValue.data;
  });
  loadVersion();
})

</script>

<template>
  <h1>{{ msg }}</h1>

  <div aria-label="result" class="result">{{ result }}</div>
  <div class="card">
    <div class="input-box">
      <input aria-label="input" class="input" v-model="name" type="text" autocomplete="off"/>
      <button aria-label="greet-btn" class="btn" @click="doGreet">Greet</button>
    </div>
  </div>

  <div class="footer">
    <div><p>Library Squirrel Wails v{{ version }}</p></div>
    <div><p>{{ time }}</p></div>
  </div>
</template>
