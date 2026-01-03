FROM node:20-alpine
WORKDIR /app

# Install dependencies
COPY package.json package-lock.json* ./
# prefer `npm ci` when lockfile is present, fall back to `npm install`
RUN npm ci --silent || npm install --silent

# Copy source
COPY . .

EXPOSE 12233

CMD ["npm", "run", "dev"]
