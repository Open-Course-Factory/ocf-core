<template>
    <div class="bubble_container">
        <div>
            <div v-for="(item, index) in schedule" :key="index" class="bubble">
                <div class="circle">{{ item.time }}</div>
                <div class="bubble_content">
                    <h1>{{ item.title }}</h1>
                    <p>{{ item.description }}</p>
                </div>
            </div>
        </div>
        <hr>
    </div>
</template>

<script>
export default {
    data() {
        return {
            schedule: [
                { time: '9h00', title: 'Début du cours', description: '(Appel Pepal !)' },
                { time: '10h30', title: 'Pause', description: '(15 minutes)' },
                { time: '12h30', title: 'Pause Déjeuner', description: '(1 heure)' },
                { time: '15h15', title: 'Pause', description: '(15 minutes)' },
                { time: '17h00', title: 'Fin du cours', description: '' }
            ]
        };
    }
};
</script>

<style scoped>
/* Ajoutez vos styles CSS ici si nécessaire */
</style>