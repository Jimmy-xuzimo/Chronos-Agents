const root=document.documentElement;
try{root.dataset.theme=localStorage.getItem('chronos-site-theme')||(matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light')}catch{root.dataset.theme='light'}
const toggle=document.querySelector('#theme-toggle');
function sync(){toggle.setAttribute('aria-label',root.dataset.theme==='dark'?'切换浅色模式':'切换深色模式');toggle.setAttribute('aria-pressed',String(root.dataset.theme==='dark'))}
toggle.addEventListener('click',()=>{root.dataset.theme=root.dataset.theme==='dark'?'light':'dark';try{localStorage.setItem('chronos-site-theme',root.dataset.theme)}catch{}sync()});sync();
if('IntersectionObserver' in window&&!matchMedia('(prefers-reduced-motion:reduce)').matches){const observer=new IntersectionObserver(items=>items.forEach(item=>{if(item.isIntersecting){item.target.classList.add('visible');observer.unobserve(item.target)}}),{threshold:.08});document.querySelectorAll('.reveal').forEach(el=>{el.classList.add('will-reveal');observer.observe(el)})}else{document.querySelectorAll('.reveal').forEach(el=>el.classList.add('visible'))}
