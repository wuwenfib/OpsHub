<script setup>
// ECharts 的薄封装：传入 option 即渲染；option 变化时重绘，容器尺寸变化时自适应，组件销毁时释放。
// 生命周期与 C++ 的 RAII 同理：mounted 时创建图表实例，unmounted 时 dispose。
import { onMounted, onUnmounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, LegendComponent, MarkLineComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

// 按需注册：只打包用到的图表类型和组件
echarts.use([BarChart, GridComponent, LegendComponent, MarkLineComponent, TooltipComponent, CanvasRenderer])

const props = defineProps({
  option: { type: Object, required: true },
  height: { type: String, default: '300px' },
})

const el = ref(null)
let chart
let observer

onMounted(() => {
  chart = echarts.init(el.value)
  chart.setOption(props.option)
  observer = new ResizeObserver(() => chart.resize())
  observer.observe(el.value)
})

// notMerge=true：分组数变少时，旧的柱子要被清掉，而不是与新数据合并
watch(
  () => props.option,
  (opt) => chart?.setOption(opt, true),
  { deep: true },
)

onUnmounted(() => {
  observer?.disconnect()
  chart?.dispose()
})
</script>

<template>
  <div ref="el" :style="{ height, width: '100%' }" />
</template>
