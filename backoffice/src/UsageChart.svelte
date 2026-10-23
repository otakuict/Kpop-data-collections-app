<script>
 import { Chart, LineController, LineElement, PointElement, BarController, BarElement, CategoryScale, LinearScale, Tooltip, Filler } from 'chart.js';
 let { labels, values, label, color='#5fd2ef', type='line', seconds=false } = $props();
 let canvas=$state();
 Chart.register(LineController, LineElement, PointElement, BarController, BarElement, CategoryScale, LinearScale, Tooltip, Filler);

 $effect(()=>{
  if(!canvas)return;
  const chart=new Chart(canvas,{
   type,
   data:{labels,datasets:[{label,data:values,borderColor:color,backgroundColor:type==='bar'?color:`${color}18`,borderWidth:2,fill:true,tension:.3,pointRadius:values.length>30?0:2,pointHoverRadius:5,borderRadius:4,maxBarThickness:28}]},
   options:{
    responsive:true,maintainAspectRatio:false,animation:false,indexAxis:type==='bar'?'y':'x',
    interaction:{intersect:false,mode:'index'},
    plugins:{legend:{display:false},tooltip:{backgroundColor:'#182c43',titleColor:'#edf4fb',bodyColor:'#c6d8e8',padding:12,displayColors:false,callbacks:{label:context=>`${label}: ${seconds?`${Math.round(Number(context.raw))}s`:Number(context.raw).toLocaleString()}`}}},
    scales:{
     x:{grid:{display:false},border:{display:false},ticks:{color:'#8fa6bf',maxTicksLimit:7,maxRotation:0,font:{size:11},...(type==='bar'?{precision:0}:{} )}},
     y:{beginAtZero:true,border:{display:false},grid:{color:'#25384c66'},ticks:{color:'#8fa6bf',font:{size:11},...(type==='line'?{precision:0}:{}),...(seconds?{callback:value=>`${value}s`}:{})}},
    },
   },
  });
  return ()=>chart.destroy();
 });
</script>

<div class="usage-chart" role="img" aria-label={label}><canvas bind:this={canvas} aria-hidden="true">Chart data is available in the table below.</canvas></div>

<style>
 .usage-chart{position:relative;width:100%;height:240px;min-width:0}
 @media(max-width:650px){.usage-chart{height:210px}}
</style>
