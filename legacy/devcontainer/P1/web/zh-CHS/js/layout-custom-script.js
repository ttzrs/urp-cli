$(document).ready( () => {
scrollSidebar();
});
/**
*  scrollSidebar scrolls the sidebar,
*  if the active item is not visible
*/
function scrollSidebar() {
var sidebar = $('.nav-site-sidebar');
var item = $('.nav-site-sidebar .active');
var target = item[0];
if(target) {
target.scrollIntoView();
}
}
